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
resource "aws_instance" "imds_server" {
  subnet_id         = var.attacker_subnet_id
  source_dest_check = false

  vpc_security_group_ids = [
    aws_security_group.allow_http.id,
    aws_security_group.allow_ssh_any.id,
    aws_security_group.allow_egress_any.id,
  ]

  ami           = "ami-0885b1f6bd170450c" // Ubuntu 20.04
  instance_type = var.instance_type

  key_name      = module.ssh_key_pair.key_name

  connection {
    type     = "ssh"
    user     = "ubuntu"
    host     = self.public_ip
    private_key = module.ssh_key_pair.private_key
  }

  provisioner "remote-exec" {
    inline = [
      "sudo apt-get update",
      "sudo apt-get install -y nginx",
    ]
  }

  provisioner "file" {
    source = "./files/nginx-imds.conf"
    destination = "/etc/nginx/sites-enabled/default"
  }

  provisioner "file" {
    source = "./files/imds/"
    destination = "/var/www/imds"
  }

  provisioner "remote-exec" {
    script = "./scripts/setup_imds_proxy.sh"
  }
}

