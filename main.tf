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

resource "aws_iam_role_policy_attachment" "network_admin" {
  policy_arn = "arn:aws:iam::aws:policy/job-function/NetworkAdministrator"
  role = aws_iam_role.network_admin.name
}

resource "aws_iam_role" "network_admin" {
  name = "NetworkAdmin"
  assume_role_policy = <<EOH
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
EOH
}

resource "aws_iam_instance_profile" "imds_server" {
  name = "NetworkAdmin"
  role = aws_iam_role.network_admin.name
}

# WARNING: do not assign a privileged roll to this instance, see notes notes below on the user_data attribute. Really
# though you shouldn't be running this at all in an account you (or anyone else) actively uses.
resource "aws_instance" "imds_server" {
  subnet_id         = var.attacker_subnet_id
  source_dest_check = false
  iam_instance_profile = aws_iam_instance_profile.imds_server.id

  vpc_security_group_ids = [
    aws_security_group.allow_http.id,
    aws_security_group.allow_ssh_any.id,
    aws_security_group.allow_egress_any.id,
  ]

  ami           = var.fake_imds_ami
  instance_type = var.instance_type

  key_name      = module.ssh_key_pair.key_name

  tags = {
    Name = "FakeIMDSServer"
  }
}

resource "null_resource" "imds_server_setup" {
  connection {
    type     = "ssh"
    user     = "ubuntu"
    host     = aws_instance.imds_server.public_ip
    private_key = module.ssh_key_pair.private_key
  }

  provisioner "remote-exec" {
    script = "./scripts/setup_imds_proxy.sh"
  }
}

resource "null_resource" "imds_server_files" {
  depends_on = [null_resource.imds_server_setup]

  connection {
    type     = "ssh"
    user     = "ubuntu"
    host     = aws_instance.imds_server.public_ip
    private_key = module.ssh_key_pair.private_key
  }

  provisioner "file" {
    source = "./files/nginx-imds.conf"
    destination = "/etc/nginx/sites-available/default"
  }

  provisioner "file" {
    source = "./files/imds"
    destination = "/var/www/"
  }
}

resource "null_resource" "imds_server_restart" {
  depends_on = [null_resource.imds_server_files]

  connection {
    type     = "ssh"
    user     = "ubuntu"
    host     = aws_instance.imds_server.public_ip
    private_key = module.ssh_key_pair.private_key
  }

  provisioner "remote-exec" {
    inline = ["sudo service nginx restart"]
  }
}

