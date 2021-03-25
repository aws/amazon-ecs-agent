#!/bin/bash
set -e # exit on failure

fail() {
    echo ""
    echo -e "\033[0;31mInstallation Failed\033[0m" # print it in red
    exit 1
}

# check if the script is run with root or sudo
if [ $(id -u) -ne 0 ]; then
    echo "Please run as root."
    fail
fi

# check if system is using systemd
if ! pidof systemd &>/dev/null; then
    echo "The install script currently supports only systemd."
    fail
fi

if [ -f "/sys/fs/cgroup/cgroup.controllers" ]; then
    echo "Your system is using cgroups v2, which is not supported by ECS."
    echo "Please change your system to cgroups v1 and reboot. If your system has grubby, we suggest using the following command:"
    echo '    sudo grubby --update-kernel=ALL --args="systemd.unified_cgroup_hierarchy=0" && sudo shutdown -r now'
    fail
fi

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
# Whether to check sha for the downloaded ecs-init package. true unless --rpm-url or --deb-url is specified.
CHECK_SHA=true
while :; do
    case "$1" in
    --region)
        REGION="$2"
        shift 2
        ;;
    --cluster)
        ECS_CLUSTER="$2"
        shift 2
        ;;
    --activation-code)
        ACTIVATION_CODE="$2"
        shift 2
        ;;
    --activation-id)
        ACTIVATION_ID="$2"
        shift 2
        ;;
    --docker-install-source)
        DOCKER_SOURCE="$2"
        shift 2
        ;;
    --ecs-version)
        ECS_VERSION="$2"
        shift 2
        ;;
    --deb-url)
        DEB_URL="$2"
        CHECK_SHA=false
        shift 2
        ;;
    --rpm-url)
        RPM_URL="$2"
        CHECK_SHA=false
        shift 2
        ;;
    --ecs-endpoint)
        ECS_ENDPOINT="$2"
        shift 2
        ;;
    --skip-registration)
        SKIP_REGISTRATION=true
        shift 1
        ;;
    *)
        [ -z "$1" ] && break
        echo "invalid option: [$1]"
        fail
        ;;
    esac
done

if [ -z "$REGION" ]; then
    echo "--region is required"
    fail
fi
# If activation code is absent and skip activation flag is present, set flag to skip ssm registration
# if both activation code is present
if $SKIP_REGISTRATION; then
    echo "Skipping registering as --skip-registration flag is specified."
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

# TODO [Update before release] Change to prod regional buckets
S3_BUCKET="amazon-ecs-agent-shadow-$REGION"
RPM_PKG_NAME="amazon-ecs-init-$ECS_VERSION.$ARCH.rpm"
DEB_PKG_NAME="amazon-ecs-init-$ECS_VERSION.$ARCH_ALT.deb"

if [ -z "$RPM_URL" ]; then
    RPM_URL="https://$S3_BUCKET.s3.amazonaws.com/$RPM_PKG_NAME"
fi
if [ -z "$DEB_URL" ]; then
    DEB_URL="https://$S3_BUCKET.s3.amazonaws.com/$DEB_PKG_NAME"
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
    yum install epel-release -y
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

INSTANCE_REGISTERED=false
check-instance-registered() {
    try "check if instance already has an SSM managed instance ID."
    SSM_REGISTRATION_FILE='/var/lib/amazon/ssm/registration'
    if [ -f ${SSM_REGISTRATION_FILE} ]; then
        MANAGED_INSTANCE_ID=$(jq -r ".ManagedInstanceID" $SSM_REGISTRATION_FILE)
        if [ ! -z $MANAGED_INSTANCE_ID ]; then
            INSTANCE_REGISTERED=true
            return
        fi
    fi
    ok
}

register-ssm-agent() {
    check-instance-registered
    if ! $INSTANCE_REGISTERED; then
        SERVICE_NAME="amazon-ssm-agent"
        systemctl stop "$SERVICE_NAME"
        amazon-ssm-agent -register -code "$ACTIVATION_CODE" -id "$ACTIVATION_ID" -region "$REGION"
        systemctl enable "$SERVICE_NAME"
        systemctl start "$SERVICE_NAME"
        echo "Instance is registered."
    else
        echo "instance registration found."
    fi
}

install-ssm-agent() {
    try "install ssm agent"
    if [ "$(systemctl is-enabled snap.amazon-ssm-agent.amazon-ssm-agent.service)" == "enabled" ]; then
        check-instance-registered
        if $INSTANCE_REGISTERED; then
            echo "Instance has been registered."
            return
        else
            echo "We currently don't support instance registration on ssm agent installed via Snap. Please ensure your instance is registered and then rerun the script with --skip-registration flag."
            fail
        fi
    elif [ "$(systemctl is-enabled amazon-ssm-agent)" == "enabled" ]; then
        echo "ssm agent is installed, checking if the instance is registered."
        check-instance-registered
        if $INSTANCE_REGISTERED; then
            echo "Instance has been registered."
            return
        else
            echo "SSM Agent is installed. The instance needs to be registered."
            register-ssm-agent
            return
        fi
    else
        local dir
        dir="$(mktemp -d)"
        local SSM_DEB_URL="https://s3.$REGION.amazonaws.com/amazon-ssm-$REGION/latest/debian_$ARCH_ALT/amazon-ssm-agent.deb"
        local SSM_RPM_URL="https://s3.$REGION.amazonaws.com/amazon-ssm-$REGION/latest/linux_$ARCH_ALT/amazon-ssm-agent.rpm"
        local SSM_DEB_PKG_NAME="ssm-agent.deb"
        local SSM_RPM_PKG_NAME="ssm-agent.rpm"

        case "$PKG_MANAGER" in
        apt)
            curl -o "$dir/$SSM_DEB_PKG_NAME" "$SSM_DEB_URL"
            curl -o "$dir/$SSM_DEB_PKG_NAME.sig" "$SSM_DEB_URL.sig"
            ssm-agent-signature-verify "$dir/$SSM_DEB_PKG_NAME.sig" "$dir/$SSM_DEB_PKG_NAME"
            dpkg -i "$dir/ssm-agent.deb"
            ;;
        dnf | yum | zypper)
            curl -o "$dir/$SSM_RPM_PKG_NAME" "$SSM_RPM_URL"
            curl -o "$dir/$SSM_RPM_PKG_NAME.sig" "$SSM_RPM_URL.sig"
            ssm-agent-signature-verify "$dir/$SSM_RPM_PKG_NAME.sig" "$dir/$SSM_RPM_PKG_NAME"
            local args="-y"
            if [[ "$PKG_MANAGER" == "zypper" ]]; then
                args="${args} --allow-unsigned-rpm"
            fi
            $PKG_MANAGER install ${args} "$dir/$SSM_RPM_PKG_NAME"
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

    # TODO [Update before release] Change this url to main repo master branch
    curl -o "$dir/amazon-ssm-agent.gpg" "https://raw.githubusercontent.com/Realmonia/amazon-ecs-init/ssmGpg/scripts/amazon-ssm-agent.gpg"
    local fp
    fp=$(gpg --quiet --with-colons --with-fingerprint "$dir/amazon-ssm-agent.gpg" | awk -F: '$1 == "fpr" {print $10;}')
    echo "$fp"
    if [ "$fp" != "8108A07A9EBE248E3F1C63F254F4F56E693ECA21" ]; then
        echo "amazon-ssm-agent GPG public key fingerprint verification fail. Stop the installation of the amazon-ssm-agent. Please contact AWS Support."
        fail
    fi
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
        # TODO upgrade the agent?
        ok
        return
    fi

    local dir
    dir="$(mktemp -d)"
    case "$PKG_MANAGER" in
    apt)
        curl -o "$dir/$DEB_PKG_NAME" "$DEB_URL"
        if $CHECK_SHA; then
            curl -o "$dir/$DEB_PKG_NAME.asc" "$DEB_URL.asc"
            ecs-init-signature-verify "$dir/$DEB_PKG_NAME.asc" "$dir/$DEB_PKG_NAME"
        fi
        apt install -y "$dir/$DEB_PKG_NAME"
        rm -rf "$dir"
        ;;
    dnf | yum | zypper)
        curl -o "$dir/$RPM_PKG_NAME" "$RPM_URL"
        if $CHECK_SHA; then
            curl -o "$dir/$RPM_PKG_NAME.asc" "$RPM_URL.asc"
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
    if [ ! -z "$ECS_ENDPOINT" ]; then
        echo "ECS_BACKEND_HOST=$ECS_ENDPOINT" >>/var/lib/ecs/ecs.config
    fi
    systemctl enable ecs
    systemctl start ecs

    ok
}

ecs-init-signature-verify() {
    try "verify the signature of amazon-ecs-init package"

    #TODO [Update before release] Update links here to use prod urls (or urls specified by $DEB_URL or $RPM_URL)
    curl -o "$dir/amazon-ecs-init.gpg" "https://ecs-init-packages-testing.s3.amazonaws.com/amazon-ecs-public-key.gpg"
    gpg --import "$dir/amazon-ecs-init.gpg"

    if gpg --verify "$1" "$2"; then
        echo "amazon-ecs-init GPG verification passed. Install amazon-ecs-init."
    else
        echo "amazon-ecs-init GPG verification failed. Stop the installation of amazon-ecs-init. Please contact AWS Support."
        fail
    fi

    ok
}

verify-agent() {
    retryLimit=10
    i=0
    for ((i = 0 ; i < retryLimit ; i++))
    do
        curlResult="$(timeout 10 curl -s http://localhost:51678/v1/metadata | jq .ContainerInstanceArn)"
        if [ ! "$curlResult" == "null" ] && [ -n "$curlResult" ]; then
            echo "Ping ECS Agent registered successfully! Container instance arn: $curlResult"
            echo ""
            echo "You can check your ECS cluster here https://console.aws.amazon.com/ecs/home?region=$REGION#/clusters/$ECS_CLUSTER"
            return
        fi
        sleep 10 # wait for 10s before next retry for agent to start up.
    done

    # TODO [Update before release] Provide hyperlink to public doc troubleshoot page
    echo "ECS Agent verification failed. Please check logs at /var/log/ecs/ecs-agent.log and follow documentation [HERE]"
    fail
}

show-license() {
    echo ""
    echo "##########################"
    echo "This setup artifact uses Apache License 2.0."
    echo "For more details, see https://github.com/aws/amazon-ecs-agent/blob/master/LICENSE"
    echo "##########################"
    echo ""
}

if ! $SKIP_REGISTRATION; then
    install-ssm-agent
fi
install-docker "$DOCKER_SOURCE"
install-ecs-agent
verify-agent
show-license
