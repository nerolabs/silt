data "aws_caller_identity" "me" {}

locals {
  az_letter  = { "0" = "a", "1" = "b", "2" = "c" }
  core_roles = ["validator", "registry", "storage"]
  name       = "silt-awstest-${var.run_id}"
}

# Amazon Linux 2023 (x86_64) — SSM agent + aws-cli preinstalled (keyless access + S3 pull).
data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]
  filter {
    name   = "name"
    values = ["al2023-ami-2023.*-x86_64"]
  }
  filter {
    name   = "architecture"
    values = ["x86_64"]
  }
}

# ── Network: one regional VPC, public subnet per AZ + a private NAT subnet ───────
resource "aws_vpc" "silt" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  tags                 = { Name = local.name }
}

resource "aws_internet_gateway" "igw" {
  vpc_id = aws_vpc.silt.id
  tags   = { Name = "${local.name}-igw" }
}

resource "aws_subnet" "public" {
  for_each                = var.public_subnets # "0"/"1"/"2" -> CIDR
  vpc_id                  = aws_vpc.silt.id
  cidr_block              = each.value
  availability_zone       = "${var.region}${local.az_letter[each.key]}"
  map_public_ip_on_launch = true
  tags                    = { Name = "${local.name}-public-${each.key}" }
}

resource "aws_subnet" "nat" {
  vpc_id            = aws_vpc.silt.id
  cidr_block        = var.nat_subnet
  availability_zone = "${var.region}${local.az_letter["0"]}"
  tags              = { Name = "${local.name}-nat" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.silt.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }
  tags = { Name = "${local.name}-public-rt" }
}

resource "aws_route_table_association" "public" {
  for_each       = aws_subnet.public
  subnet_id      = each.value.id
  route_table_id = aws_route_table.public.id
}

# Managed NAT gateway (replaces the GCP natgw silt node) so natted nodes reach SSM/S3.
resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = { Name = "${local.name}-nat-eip" }
}

resource "aws_nat_gateway" "nat" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public["0"].id
  tags          = { Name = "${local.name}-natgw" }
  depends_on    = [aws_internet_gateway.igw]
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.silt.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat.id
  }
  tags = { Name = "${local.name}-private-rt" }
}

resource "aws_route_table_association" "nat" {
  subnet_id      = aws_subnet.nat.id
  route_table_id = aws_route_table.private.id
}

# ── Security group: swarm/relay/registry from anywhere (real field net) + all
#    intra-VPC (cross-AZ) + egress for SSM/S3/internet ─────────────────────────────
resource "aws_security_group" "silt" {
  name_prefix = "${local.name}-"
  vpc_id      = aws_vpc.silt.id

  ingress {
    description = "silt swarm"
    from_port   = var.swarm_port
    to_port     = var.swarm_port
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    description = "silt relay"
    from_port   = var.relay_port
    to_port     = var.relay_port
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    description = "silt registry (HTTPS)"
    from_port   = var.registry_port
    to_port     = var.registry_port
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    description = "all intra-VPC (cross-AZ mesh)"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
  }
  egress {
    description = "SSM + S3 + internet"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = { Name = "${local.name}-sg" }
}

# ── IAM: instance profile granting SSM (keyless access) + read of the binary bucket ─
resource "aws_iam_role" "node" {
  name_prefix = "silt-aws-"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17",
    Statement = [{ Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
  tags = { Name = "${local.name}-role" }
}

resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_role_policy" "s3read" {
  name = "silt-s3read"
  role = aws_iam_role.node.id
  policy = jsonencode({
    Version = "2012-10-17",
    Statement = [{
      Effect   = "Allow",
      Action   = ["s3:GetObject"],
      Resource = ["${aws_s3_bucket.art.arn}/*"]
    }]
  })
}

resource "aws_iam_instance_profile" "node" {
  name_prefix = "silt-aws-"
  role        = aws_iam_role.node.name
}

# ── Artifacts bucket + the silt binary ──────────────────────────────────────────
resource "aws_s3_bucket" "art" {
  bucket        = "${local.name}-${data.aws_caller_identity.me.account_id}"
  force_destroy = true
  tags          = { Name = "${local.name}-artifacts" }
}

resource "aws_s3_object" "silt" {
  bucket = aws_s3_bucket.art.id
  key    = "silt-linux-amd64"
  source = var.silt_binary_path
  etag   = filemd5(var.silt_binary_path)
}

# ── The fleet: one EC2 per non-natgw node ───────────────────────────────────────
resource "aws_instance" "node" {
  for_each = var.nodes

  ami           = data.aws_ami.al2023.id
  instance_type = var.instance_type
  subnet_id     = each.value.role == "natted" ? aws_subnet.nat.id : aws_subnet.public[tostring(each.value.az_slot)].id
  private_ip    = each.value.ip

  vpc_security_group_ids      = [aws_security_group.silt.id]
  iam_instance_profile        = aws_iam_instance_profile.node.name
  associate_public_ip_address = each.value.role == "natted" ? false : true

  # Cost backstop: user-data runs `shutdown -h +ttl`; terminate (not stop) on shutdown.
  instance_initiated_shutdown_behavior = "terminate"

  root_block_device {
    volume_size = var.boot_disk_gb
    volume_type = "gp3"
  }

  # Spot for non-core, unless all_on_demand (a CERT run — Spot preemption is fatal).
  # Core roles stay on-demand when core_on_demand=true (the GCP lesson).
  dynamic "instance_market_options" {
    for_each = (var.all_on_demand || (var.core_on_demand && contains(local.core_roles, each.value.role))) ? [] : [1]
    content {
      market_type = "spot"
      spot_options {
        instance_interruption_behavior = "terminate"
        spot_instance_type             = "one-time"
      }
    }
  }

  user_data = templatefile("${path.module}/../provision/silt-startup.sh", {
    node_name     = each.key
    argv          = each.value.argv
    role          = each.value.role
    s3_uri        = "s3://${aws_s3_bucket.art.bucket}/${aws_s3_object.silt.key}"
    ttl_minutes   = var.ttl_minutes
    registry_port = var.registry_port
  })

  tags = {
    Name        = "${local.name}-${each.key}"
    "silt:node" = each.key
    "silt:role" = each.value.role
  }
}
