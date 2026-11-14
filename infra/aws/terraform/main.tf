# One EC2 host running the compose stack. Local state; test env only.
terraform {
  required_version = ">= 1.6"
  required_providers {
    aws   = { source = "hashicorp/aws", version = "~> 6.0" }
    http  = { source = "hashicorp/http", version = "~> 3.4" }
    local = { source = "hashicorp/local", version = "~> 2.5" }
  }
}

variable "aws_profile" {
  type    = string
  default = "crayo"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "instance_type" {
  type    = string
  default = "t4g.small" # arm64, free-tier; images are built on the (arm64) Mac, host only runs them
}

variable "ssh_public_key_path" {
  type    = string
  default = "~/.ssh/id_ed25519.pub"
}

variable "ssh_cidr" {
  type        = string
  default     = ""
  description = "who may SSH in; empty = your current public IP /32"
}

variable "domain" {
  type        = string
  default     = ""
  description = "base domain pointed at the EIP (needs *.domain too); empty = <ip>.sslip.io"
}

provider "aws" {
  profile = var.aws_profile
  region  = var.region
  default_tags { tags = { Project = "nearly" } }
}

data "http" "my_ip" {
  url = "https://checkip.amazonaws.com"
}

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-arm64-server-*"]
  }
}

locals {
  ssh_cidr = var.ssh_cidr != "" ? var.ssh_cidr : "${chomp(data.http.my_ip.response_body)}/32"
  domain   = var.domain != "" ? var.domain : "${replace(aws_eip.this.public_ip, ".", "-")}.sslip.io"
}

resource "aws_key_pair" "this" {
  key_name_prefix = "nearly-"
  public_key      = file(pathexpand(var.ssh_public_key_path))
}

# default VPC: nothing here needs private subnets
resource "aws_security_group" "this" {
  name_prefix = "nearly-"
  description = "nearly: ssh from operator, http/https from anywhere"

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [local.ssh_cidr]
  }
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "this" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  key_name               = aws_key_pair.this.key_name
  vpc_security_group_ids = [aws_security_group.this.id]

  root_block_device {
    volume_size = 30
    volume_type = "gp3"
    encrypted   = true
  }
  metadata_options { http_tokens = "required" }
  tags = { Name = "nearly" }
}

resource "aws_eip" "this" {
  instance = aws_instance.this.id
  domain   = "vpc"
}

resource "local_file" "inventory" {
  filename        = "${path.module}/../ansible/inventory.ini"
  file_permission = "0644"
  content         = <<-EOT
    [nearly]
    ${aws_eip.this.public_ip} ansible_user=ubuntu domain=${local.domain}
  EOT
}

output "ip" { value = aws_eip.this.public_ip }
output "url" { value = "https://${local.domain}" }
output "ssh" { value = "ssh ubuntu@${aws_eip.this.public_ip}" }
