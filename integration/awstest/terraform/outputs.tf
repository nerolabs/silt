# A single machine-readable map the orchestrator consumes: role/az/instance-id +
# private/public IP per node. awstest reaches every node — natted included — over
# `aws ssm start-session` (keyless), so it addresses by INSTANCE_ID, not by IP/keys.

output "nodes" {
  value = {
    for k, inst in aws_instance.node : k => {
      instance_id = inst.id
      role        = var.nodes[k].role
      az          = inst.availability_zone
      internal_ip = var.nodes[k].ip
      external_ip = inst.public_ip # "" for natted nodes
    }
  }
}

output "artifacts_bucket" {
  value = aws_s3_bucket.art.bucket
}

output "run_id" {
  value = var.run_id
}
