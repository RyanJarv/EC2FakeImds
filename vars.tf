# Configure the AWS Provider
variable "region" {
  description = "Region to deploy resources in."
}

variable "instance_type" {
  type = string
  default = "t2.small"
  description = "Attack node instance size."
}

variable "vpc_id" {
  type = string
  description = <<EOH
  WARNING: Do not run this in an account or vpc you or someone else actively uses. (Unless of course your fine with
  things potentially breaking for a bit).

  All instances in subnets other then attacker_subnet_id subnet in this VPC will use the fake IMDS server for it's
  initial IMDS server on creation. After the user-data served from this node executes the other subnets routing will
  be set back to normal and re-init'd with their real IMDS server.
EOH
}

variable "attacker_subnet_id" {
  type = string
  description = <<EOH
  This is the subnet where the attacker's instance is spun up. It's the only subnet in the VPC that will never be
  affected by routing changes when a node is spun up in it. This is because the attacker node still needs to talk to
  it's own IMDS server when it is serving other recently spun up instances.

EOH
}

variable "fake_imds_ami" {
  default = "ami-0885b1f6bd170450c" // Ubuntu 20.04
  description = "AMI to use for the fake imds instance"
}
