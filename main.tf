terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 3.0"
    }
  }
}

provider "aws" {
  region = var.region
}


module "ssh_key_pair" {
  source                = "git::https://github.com/cloudposse/terraform-aws-key-pair.git?ref=master"
  namespace             = "jn"
  stage                 = "sandbox"
  name                  = "fake_imds"
  ssh_public_key_path   = pathexpand("~/.ssh")
  generate_ssh_key      = "true"
  private_key_extension = ".pem"
  public_key_extension  = ".pub"
}

# WARNING: do not assign a privileged roll to this instance, see notes notes below on the user_data attribute. Really
# though you shouldn't be running this at all in an account you (or anyone else) actively uses.
module "instance" {
  source                      = "git::https://github.com/cloudposse/terraform-aws-ec2-instance.git?ref=master"
  ssh_key_pair                = module.ssh_key_pair.key_name
  instance_type               = var.instance_type
  vpc_id                      = var.vpc_id
  security_groups             = [aws_security_group.allow_http.id]
  subnet                      = data.aws_subnet.attacker_subnet.id
  name                        = "ec2"
  namespace                   = "jn"
  stage                       = "sandbox"
  source_dest_check           = false
  allowed_ports               = ["80"]

  # This is a dirty hack to allow us to serve traffic for 169.254.169.254 as well as use our own IMDS server at
  # 169.254.169.254 over eth0. Note that we use 169.254.168.254 to for the nginx listen IP not 169.254.169.254 despite
  # it appearing as the second from other hosts. i.e everyone but us get's the fake IMDS server.
  #
  # Reaching our own IMDS is important for normal functioning of the instance, but we also use it as a fallback in nginx
  # to mock out values. This means that other nodes are accessing our real IMDS server in cases we didn't care to mock
  # out in nginx.
  user_data = <<EOH
#!/usr/bin/env bash
set -euo pipefail

ip addr add 169.254.168.254/32 dev eth0
iptables -t nat -A PREROUTING -s 169.254.168.254,172.31.54.46 -d 169.254.169.254/32 -j RETURN
iptables -t nat -A PREROUTING -d 169.254.169.254/32 -j DNAT --to-destination 169.254.168.254
iptables -t nat -A POSTROUTING -s 169.254.168.254 -d 169.254.169.254/32 -o eth0 -j MASQUERADE
service nginx restart
EOH

}
