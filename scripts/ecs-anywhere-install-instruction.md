## How to run:

```
# First get an activation code.
#   This can be run anywhere, and you should save the outputted activation ID and code
aws ssm create-activation --default-instance-name myName --iam-role $SSM_ECS_INSTANCE_ROLE --region us-west-2 --registration-limit 100

# Second run this script on instance:
sudo ./ecs-anywhere-install.sh --region us-west-2 --cluster $CLUSTER --activation-id "$ACTIVATION_ID" --activation-code "$ACTIVATION_CODE"
# 
```
These are the different flags available: 

```
Usage of ./ecs-anywhere-install.sh
  --region string
    	(required) this defines the region where the ssm registration is present andd where the ecs agent will find the cluster to connect to.
  --cluster string
    	(optional) pass the cluster name that ECS agent would connect too. By default its value is 'default'
  --activation-id string
    	(optional) activation id from the create activation command.
  --activation-code string
    	(optional) activation code from the create activation command.
  --docker-install-source
        (optional) Possible values are 'all, distro, none'
  --deb-url string
    	(optional) deb url for ecs agent deb package. Applicable to deb-based systems (ubuntu, debian). If not specified, defaulting to the location of the official ecs agent deb package.
  --rpm-url string
    	(optional) rpm url for ecs agent rpm package. Applicable to rpm-based systems (centos, fedora, suse). If not specified, defaulting to the location of the official ecs agent rpm package.
  --ecs-version string
    	(optional) Version of the ecs agent rpm/deb package to use. If not specified, default to latest.
  --ecs-endpoint string
    	(optional) ecs endpoint that can be used to different requests to ECS internal endpoints
  --skip-registration
    	(optional) if this is enabled, ssm agent install and instance registration with ssm is skipped
```