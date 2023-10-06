#!/bin/bash
set -e # exit on failure

fail() {
    echo ""
    echo -e "\033[0;31mInstallation Failed\033[0m" # print it in red
    exit 1
}

check-option-value() {
    if [ "${2:0:2}" == "--" ]; then
        echo "Option $1 was passed an invalid value: $2. Perhaps you passed in an empty env var?"
        fail
    fi
}

usage() {
    echo "$(basename "$0") [--help] --region REGION --activation-code CODE --activation-id ID [--cluster CLUSTER] [--enable-gpu] [--docker-install-source all|docker|distro|none] [--ecs-version VERSION] [--ecs-endpoint ENDPOINT] [--skip-registration] [--no-start]

  --help
        (optional) display this help message.
  --region string
        (required) this must match the region of your ECS cluster and SSM activation.
  --activation-id string
        (required) activation id from the create activation command. Not required if --skip-registration is specified.
  --activation-code string
        (required) activation code from the create activation command. Not required if --skip-registration is specified.
  --cluster string
        (optional) pass the cluster name that ECS agent will connect too. By default its value is 'default'.
  --enable-gpu
        (optional) if this flag is provided, GPU support for ECS will be enabled.
  --docker-install-source
        (optional) Source of docker installation. Possible values are 'all, docker, distro, none'. Defaults to 'all'.
  --ecs-version string
        (optional) Version of the ECS agent rpm/deb package to use. If not specified, default to latest.
  --skip-registration
        (optional) if this is enabled, SSM agent install and instance registration with SSM is skipped.
  --certs-file
        (optional) TLS certs for execute command feature. Defaults to searching for certs in known possible locations.
  --no-start
        (optional) if this flag is provided, SSM agent, docker and ECS agent will not be started by the script despite being installed."
}

# required:
REGION=""
ACTIVATION_CODE=""
ACTIVATION_ID=""
# optional:
SKIP_REGISTRATION=false
ECS_CLUSTER=""
DOCKER_SOURCE=""
ECS_VERSION=""
DEB_URL=""
RPM_URL=""
ECS_ENDPOINT=""
CERTS_FILE=""
# Whether to check signature for the downloaded amazon-ecs-init package. true unless --skip-gpg-check
# specified. --skip-gpg-check is mostly for testing purpose (so that we can test a custom build of ecs init package
# without having to sign it).
CHECK_SIG=true
NO_START=false
while :; do
    case "$1" in
    --help)
        usage
        exit 0
        ;;
    --region)
        check-option-value "$1" "$2"
        REGION="$2"
        shift 2
        ;;
    --cluster)
        check-option-value "$1" "$2"
        ECS_CLUSTER="$2"
        shift 2
        ;;
    --activation-code)
        check-option-value "$1" "$2"
        ACTIVATION_CODE="$2"
        shift 2
        ;;
    --activation-id)
        check-option-value "$1" "$2"
        ACTIVATION_ID="$2"
        shift 2
        ;;
    --docker-install-source)
        check-option-value "$1" "$2"
        DOCKER_SOURCE="$2"
        shift 2
        ;;
    --ecs-version)
        check-option-value "$1" "$2"
        ECS_VERSION="$2"
        shift 2
        ;;
    --deb-url)
        check-option-value "$1" "$2"
        DEB_URL="$2"
        shift 2
        ;;
    --rpm-url)
        check-option-value "$1" "$2"
        RPM_URL="$2"
        shift 2
        ;;
    --ecs-endpoint)
        check-option-value "$1" "$2"
        ECS_ENDPOINT="$2"
        shift 2
        ;;
    --certs-file)
        check-option-value "$1" "$2"
        CERTS_FILE="$2"
        shift 2
        ;;
    --skip-registration)
        SKIP_REGISTRATION=true
        shift 1
        ;;
    --enable-gpu)
        GPU_ENABLED=true
        shift 1
        ;;
    --no-start)
        NO_START=true
        shift 1
        ;;
    --skip-gpg-check)
        CHECK_SIG=false
        shift 1
        ;;
    *)
        [ -z "$1" ] && break
        echo "invalid option: [$1]"
        fail
        ;;
    esac
done

# check if the script is run with root or sudo
if [ $(id -u) -ne 0 ]; then
    echo "Please run as root."
    fail
fi

# check if system is using systemd
# from https://www.freedesktop.org/software/systemd/man/sd_booted.html
if [ ! -d /run/systemd/system ]; then
    echo "The install script currently supports only systemd."
    fail
fi

SSM_SERVICE_NAME="amazon-ssm-agent"
SSM_BIN_NAME="amazon-ssm-agent"
if systemctl is-enabled snap.amazon-ssm-agent.amazon-ssm-agent.service &>/dev/null; then
    echo "Detected SSM agent installed via snap."
    SSM_SERVICE_NAME="snap.amazon-ssm-agent.amazon-ssm-agent.service"
    SSM_BIN_NAME="/snap/amazon-ssm-agent/current/amazon-ssm-agent"
fi
SSM_MANAGED_INSTANCE_ID=""

if [ -z "$REGION" ]; then
    echo "--region is required"
    fail
fi
# If activation code is absent and skip activation flag is present, set flag to skip ssm registration
# if both activation code is present
if $SKIP_REGISTRATION; then
    echo "Skipping ssm registration."
    if ! systemctl is-enabled $SSM_SERVICE_NAME &>/dev/null; then
        echo "--skip-registration flag specified but the SSM agent service is not running."
        echo "a running SSM agent service is required for ECS Anywhere."
        fail
    fi
else
    if [[ -z $ACTIVATION_ID || -z $ACTIVATION_CODE ]]; then
        echo "Both --activation-id and --activation-code are required unless --skip-registration is specified."
        fail
    fi
fi
if [ -z "$ECS_CLUSTER" ]; then
    ECS_CLUSTER="default"
fi
if [ -z "$DOCKER_SOURCE" ]; then
    DOCKER_SOURCE="all"
fi
if [ -z "$ECS_VERSION" ]; then
    ECS_VERSION="latest"
fi

ARCH=$(uname -m)
if [ "$ARCH" == "x86_64" ]; then
    ARCH_ALT="amd64"
elif [ "$ARCH" == "aarch64" ]; then
    ARCH_ALT="arm64"
else
    echo "Unsupported architecture: $ARCH"
    fail
fi

S3_BUCKET="amazon-ecs-agent-$REGION"
RPM_PKG_NAME="amazon-ecs-init-$ECS_VERSION.$ARCH.rpm"
DEB_PKG_NAME="amazon-ecs-init-$ECS_VERSION.$ARCH_ALT.deb"
S3_URL_SUFFIX=""
if grep -q "^cn-" <<< "$REGION"; then
    S3_URL_SUFFIX=".cn"
fi
S3_URL="https://s3.${REGION}.amazonaws.com${S3_URL_SUFFIX}"
SSM_S3_BUCKET="amazon-ssm-$REGION"

if [ -z "$RPM_URL" ]; then
    RPM_URL="${S3_URL}/${S3_BUCKET}/$RPM_PKG_NAME"
fi
if [ -z "$DEB_URL" ]; then
    DEB_URL="${S3_URL}/${S3_BUCKET}/$DEB_PKG_NAME"
fi

# source /etc/os-release to get the VERSION_ID and ID fields
source /etc/os-release
echo "Running ECS install script on $ID $VERSION_ID"
echo "###"
echo ""
DISTRO=""
if echo "$ID" | grep ubuntu; then
    DISTRO="ubuntu"
elif echo "$ID" | grep debian; then
    DISTRO="debian"
elif echo "$ID" | grep fedora; then
    DISTRO="fedora"
elif echo "$ID" | grep centos; then
    DISTRO="centos"
elif echo "$ID" | grep rhel; then
    DISTRO="rhel"
elif [[ "$ID" == "amzn" && "$VERSION_ID" == "2" ]]; then
    DISTRO="al2"
fi

if [ "$DISTRO" == "rhel" ]; then
    if [ ! "$DOCKER_SOURCE" == "none" ]; then
        echo "Docker install is not supported on RHEL. Please install yourself and rerun with --docker-install-source none"
        fail
    fi
fi

ok() {
    echo ""
    echo "# ok"
    echo "##########################"
    echo ""
}

try() {
    local action=$*
    echo ""
    echo "##########################"
    echo "# Trying to $action ... "
    echo ""
}

PKG_MANAGER=""
if [ -x "$(command -v dnf)" ]; then
    PKG_MANAGER="dnf"
    dnf install -y jq
elif [ -x "$(command -v yum)" ]; then
    PKG_MANAGER="yum"
    if [ "$DISTRO" != "al2" ]; then
        yum install epel-release -y
    fi
    yum install -y jq
elif [ -x "$(command -v apt)" ]; then
    PKG_MANAGER="apt"
    try "Update apt repos"
    apt update -y
    apt-get install -y curl jq
    ok
elif [ -x "$(command -v zypper)" ]; then
    PKG_MANAGER="zypper"
    zypper install -y jq
else
    echo "Unsupported package manager. Could not find dnf, yum, or apt."
    fail
fi

get-ssm-managed-instance-id() {
    SSM_REGISTRATION_FILE='/var/lib/amazon/ssm/Vault/Store/RegistrationKey'
    if [ -f ${SSM_REGISTRATION_FILE} ]; then
        SSM_MANAGED_INSTANCE_ID=$(jq -r ".instanceID" $SSM_REGISTRATION_FILE)
    fi
}

curl-helper() {
    if ! curl -o "$1" "$2" -fSs; then
        echo "Failed to download $2"
        fail
    fi
}

register-ssm-agent() {
    try "Register SSM agent"
    get-ssm-managed-instance-id
    if [ -z "$SSM_MANAGED_INSTANCE_ID" ]; then
        systemctl stop "$SSM_SERVICE_NAME" &>/dev/null
        $SSM_BIN_NAME -register -code "$ACTIVATION_CODE" -id "$ACTIVATION_ID" -region "$REGION"
        systemctl enable "$SSM_SERVICE_NAME"
        if ! $NO_START; then
            systemctl start "$SSM_SERVICE_NAME"
        else
            echo "Skip starting ssm agent because --no-start is specified."
        fi
        echo "SSM agent has been registered."
    else
        echo "SSM agent is already registered. Managed instance ID: $SSM_MANAGED_INSTANCE_ID"
    fi
    ok
}

install-ssm-agent() {
    try "install ssm agent"
    if systemctl is-enabled $SSM_SERVICE_NAME &>/dev/null; then
        echo "SSM agent is already installed."
    else
        local dir
        dir="$(mktemp -d)"
        local SSM_DEB_URL="${S3_URL}/${SSM_S3_BUCKET}/latest/debian_$ARCH_ALT/amazon-ssm-agent.deb"
        local SSM_RPM_URL="${S3_URL}/${SSM_S3_BUCKET}/latest/linux_$ARCH_ALT/amazon-ssm-agent.rpm"
        local SSM_DEB_PKG_NAME="ssm-agent.deb"
        local SSM_RPM_PKG_NAME="ssm-agent.rpm"

        case "$PKG_MANAGER" in
        apt)
            curl-helper "$dir/$SSM_DEB_PKG_NAME" "$SSM_DEB_URL"
            curl-helper "$dir/$SSM_DEB_PKG_NAME.sig" "$SSM_DEB_URL.sig"
            ssm-agent-signature-verify "$dir/$SSM_DEB_PKG_NAME.sig" "$dir/$SSM_DEB_PKG_NAME"
            chmod -R a+rX "$dir"
            dpkg -i "$dir/ssm-agent.deb"
            ;;
        dnf | yum | zypper)
            curl-helper "$dir/$SSM_RPM_PKG_NAME" "$SSM_RPM_URL"
            curl-helper "$dir/$SSM_RPM_PKG_NAME.sig" "$SSM_RPM_URL.sig"
            ssm-agent-signature-verify "$dir/$SSM_RPM_PKG_NAME.sig" "$dir/$SSM_RPM_PKG_NAME"
            local args=""
            local install_args="-y"
            if [[ "$PKG_MANAGER" == "zypper" ]]; then
                install_args="${install_args} --allow-unsigned-rpm"
                args="--no-gpg-checks"
            fi
            $PKG_MANAGER ${args} install ${install_args} "$dir/$SSM_RPM_PKG_NAME"
            ;;
        esac
        rm -rf "$dir"
    fi
    # register the instance
    register-ssm-agent
    ok
}

ssm-agent-signature-verify() {
    try "verify the signature of amazon-ssm-agent package"
    if ! command -v gpg; then
        echo "WARNING: gpg command not available on this server, not able to verify amazon-ssm-agent package signature."
        ok
        return
    fi

    curl-helper "$dir/amazon-ssm-agent.gpg" "https://raw.githubusercontent.com/aws/amazon-ecs-agent/master/scripts/amazon-ssm-agent.gpg"
    gpg --import "$dir/amazon-ssm-agent.gpg"

    if gpg --verify "$1" "$2"; then
        echo "amazon-ssm-agent GPG verification passed. Install the amazon-ssm-agent."
    else
        echo "amazon-ssm-agent GPG verification failed. Stop the installation of the amazon-ssm-agent. Please contact AWS Support."
        fail
    fi

    ok
}

# order of operations:
# all->docker->distro->none
install-docker() {
    if [ -x "$(command -v docker)" ]; then
        echo "docker is already installed, skipping installation"
        return
    fi

    case "$1" in
    all)
        install-docker "docker"
        return
        ;;
    docker)
        try "install docker from docker repos"
        case "$DISTRO" in
        centos)
            # TODO use dnf on centos if available
            yum install -y yum-utils
            yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
            yum install -y docker-ce docker-ce-cli containerd.io
            ;;
        fedora)
            dnf install -y dnf-plugins-core
            dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
            dnf install -y docker-ce docker-ce-cli containerd.io
            ;;
        debian)
            apt install -y apt-transport-https ca-certificates gnupg-agent software-properties-common
            curl -fsSL https://download.docker.com/linux/debian/gpg | sudo apt-key add -
            add-apt-repository \
                "deb [arch=$ARCH_ALT] https://download.docker.com/linux/debian \
              $(lsb_release -cs) \
              stable"
            apt update -y
            apt install -y docker-ce docker-ce-cli containerd.io
            ;;
        ubuntu)
            apt install -y apt-transport-https ca-certificates gnupg-agent software-properties-common
            curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
            add-apt-repository \
                "deb [arch=$ARCH_ALT] https://download.docker.com/linux/ubuntu \
              $(lsb_release -cs) \
              stable"
            apt update -y
            apt install -y docker-ce docker-ce-cli containerd.io
            ;;
        *)
            echo "Docker install repos not supported for this distro, trying distro install"
            install-docker "distro"
            return
            ;;
        esac
        ;;
    distro)
        try "install docker from distribution repos"

        # centos and fedora enable selinux by default in their distro installs
        case "$DISTRO" in
        centos | fedora)
            if [ "$VERSION_ID" == "8" ]; then
                echo "--docker-install-source distro is not supported on centos 8 because docker package is not available."
            else
                echo "--docker-install-source distro is not supported on $DISTRO $VERSION_ID because distro enables selinux by default."
            fi
            echo "We suggest installing using '--docker-install-source docker' or installing docker yourself before running this script."
            fail
            ;;
        esac

        case "$PKG_MANAGER" in
        dnf)
            dnf install -y docker
            ;;
        yum)
            yum install -y docker
            ;;
        apt)
            apt install -y docker.io
            ;;
        zypper)
            zypper install -y docker
            ;;
        esac
        ;;
    none)
        echo "Docker install source is none, not installing docker"
        ;;
    esac

    ok
}

install-ecs-agent() {
    try "install ecs agent"
    if [ -x "/usr/libexec/amazon-ecs-init" ]; then
        echo "ecs agent is already installed"
        ok
        return
    fi

    local dir
    dir="$(mktemp -d)"
    case "$PKG_MANAGER" in
    apt)
        curl-helper "$dir/$DEB_PKG_NAME" "$DEB_URL"
        if $CHECK_SIG; then
            curl-helper "$dir/$DEB_PKG_NAME.asc" "$DEB_URL.asc"
            ecs-init-signature-verify "$dir/$DEB_PKG_NAME.asc" "$dir/$DEB_PKG_NAME"
        fi
        chmod -R a+rX "$dir"
        apt install -y "$dir/$DEB_PKG_NAME"
        rm -rf "$dir"
        ;;
    dnf | yum | zypper)
        curl-helper "$dir/$RPM_PKG_NAME" "$RPM_URL"
        if $CHECK_SIG; then
            curl-helper "$dir/$RPM_PKG_NAME.asc" "$RPM_URL.asc"
            ecs-init-signature-verify "$dir/$RPM_PKG_NAME.asc" "$dir/$RPM_PKG_NAME"
        fi
        local args="-y"
        if [[ "$PKG_MANAGER" == "zypper" ]]; then
            args="${args} --allow-unsigned-rpm"
        fi
        $PKG_MANAGER install ${args} "$dir/$RPM_PKG_NAME"
        rm -rf "$dir"
        ;;
    esac

    if [ ! -f "/etc/ecs/ecs.config" ]; then
        touch /etc/ecs/ecs.config
    else
        echo "/etc/ecs/ecs.config already exists, preserving existing config and appending cluster name."
    fi
    echo "ECS_CLUSTER=$ECS_CLUSTER" >>/etc/ecs/ecs.config

    if [ ! -f "/var/lib/ecs/ecs.config" ]; then
        touch /var/lib/ecs/ecs.config
    else
        echo "/var/lib/ecs/ecs.config already exists, preserving existing config and appending ECS anywhere requirements."
    fi
    echo "AWS_DEFAULT_REGION=$REGION" >>/var/lib/ecs/ecs.config
    echo "ECS_EXTERNAL=true" >>/var/lib/ecs/ecs.config
    if [ -n "$ECS_ENDPOINT" ]; then
        echo "ECS_BACKEND_HOST=$ECS_ENDPOINT" >>/var/lib/ecs/ecs.config
    fi
    if [ -n "$GPU_ENABLED" ]; then
        if ! nvidia-smi &>/dev/null; then
            echo "Failed to detect GPU with nvidia-smi command. Make sure that GPUs are attached and the latest NVIDIA driver is installed and running."
            fail
        fi
        echo "ECS_ENABLE_GPU_SUPPORT=true" >>/var/lib/ecs/ecs.config
    fi
    systemctl enable ecs
    if ! $NO_START; then
        systemctl start ecs
    else
        echo "Skip starting ecs agent because --no-start is specified."
    fi

    ok
}

ecs-init-signature-verify() {
    try "verify the signature of amazon-ecs-init package"
    if ! command -v gpg; then
        echo "WARNING: gpg command not available on this server, not able to verify amazon-ecs-init package signature."
        ok
        return
    fi

    curl-helper "$dir/amazon-ecs-agent.gpg" "https://raw.githubusercontent.com/aws/amazon-ecs-agent/master/scripts/amazon-ecs-agent.gpg"
    gpg --import "$dir/amazon-ecs-agent.gpg"
    if gpg --verify "$1" "$2"; then
        echo "amazon-ecs-init GPG verification passed. Install amazon-ecs-init."
    else
        echo "amazon-ecs-init GPG verification failed. Stop the installation of amazon-ecs-init. Please contact AWS Support."
        fail
    fi

    ok
}

wait-agent-start() {
    if $NO_START; then
        echo "--no-start is specified. Not verifying ecs agent startup."
        return
    fi
    try "wait for ECS agent to start"

    retryLimit=10
    i=0
    for ((i = 0 ; i < retryLimit ; i++))
    do
        curlResult="$(timeout 10 curl -s http://localhost:51678/v1/metadata | jq .ContainerInstanceArn)"
        if [ ! "$curlResult" == "null" ] && [ -n "$curlResult" ]; then
            echo "Ping ECS Agent registered successfully! Container instance arn: $curlResult"
            echo ""
            echo "You can check your ECS cluster here https://console.aws.amazon.com/ecs/home?region=$REGION#/clusters/$ECS_CLUSTER"
            ok
            return
        fi
        sleep 10 # wait for 10s before next retry for agent to start up.
    done

    # TODO Update to ecs anywhere specific documentation when available.
    echo "Timed out waiting for ECS Agent to start. Please check logs at /var/log/ecs/ecs-agent.log and follow troubleshooting documentation at https://docs.aws.amazon.com/AmazonECS/latest/developerguide/troubleshooting.html"
    fail
}

exec-setup() {
    find-copy-certs-exec
    download-ssm-binaries-exec
}

find-copy-certs-exec() {
    # Reference for locations: https://golang.org/src/crypto/x509/root_linux.go
    CERTS_FILES_LOCATIONS=("/etc/ssl/certs/ca-certificates.crt"
      "/etc/pki/tls/certs/ca-bundle.crt"
      "/etc/ssl/ca-bundle.pem"
      "/etc/pki/tls/cacert.pem"
      "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
      "/etc/ssl/cert.pem"
    )
    CERTS_PATH="/var/lib/ecs/deps/execute-command/certs"

    echo "Copying certs for exec feature"

    # Determine certs file
    certs=""
    if [ -n "$CERTS_FILE" ] && [ -f "$CERTS_FILE" ]; then
        certs=$CERTS_FILE
    else
        if [ -n "$CERTS_FILE" ]; then
            echo "Provided certs file does not exist, looking for certs in known possible locations"
        fi
        for f in "${CERTS_FILES_LOCATIONS[@]}"
        do
            if  [ -f "$f" ]; then
                certs=$f
                break
            fi
        done
    fi

    # Copy certs to exec directory
    if [ -z "$certs" ]; then
        echo "Could not find certificates. Please rerun with --certs-file and provide a valid path"
        fail
    else
        echo "Using $certs"
        openssl verify -x509_strict "$certs"
        mkdir -p $CERTS_PATH
        cp "$certs" $CERTS_PATH/tls-ca-bundle.pem
        chmod 400 $CERTS_PATH/tls-ca-bundle.pem
    fi
    ok
}

download-ssm-binaries-exec() {
    BINARY_VERSION="3.2.1630.0"
    BINARY_PATH="/var/lib/ecs/deps/execute-command/bin/${BINARY_VERSION}"
    BINARY_DOWNLOAD_PATH="ssm-binaries"

    # Download SSM binaries from S3
    echo "Downloading SSM binaries for exec feature"

    mkdir -p $BINARY_DOWNLOAD_PATH
    curl "${S3_URL}/${SSM_S3_BUCKET}/${BINARY_VERSION}/linux_$ARCH_ALT/amazon-ssm-agent-binaries.tar.gz" -o ${BINARY_DOWNLOAD_PATH}/amazon-ssm-agent.tar.gz
    tar -xvf ${BINARY_DOWNLOAD_PATH}/amazon-ssm-agent.tar.gz -C ${BINARY_DOWNLOAD_PATH}/

    # Copy binaries to exec directory
    mkdir -p ${BINARY_PATH}
    cp ${BINARY_DOWNLOAD_PATH}/amazon-ssm-agent ${BINARY_PATH}/amazon-ssm-agent
    cp ${BINARY_DOWNLOAD_PATH}/ssm-agent-worker ${BINARY_PATH}/ssm-agent-worker
    cp ${BINARY_DOWNLOAD_PATH}/ssm-session-worker ${BINARY_PATH}/ssm-session-worker
    rm -rf ${BINARY_DOWNLOAD_PATH}
    ok
}

show-license() {
    echo ""
    echo "##########################"
    echo "This script installed three open source packages that all use Apache License 2.0."
    echo "You can view their license information here:"
    echo "  - ECS Agent https://github.com/aws/amazon-ecs-agent/blob/master/LICENSE"
    echo "  - SSM Agent https://github.com/aws/amazon-ssm-agent/blob/master/LICENSE"
    echo "  - Docker engine https://github.com/moby/moby/blob/master/LICENSE"
    echo "##########################"
    echo ""
}

if ! $SKIP_REGISTRATION; then
    install-ssm-agent
fi
install-docker "$DOCKER_SOURCE"
exec-setup
install-ecs-agent
wait-agent-start
show-license
