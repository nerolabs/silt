variable "region" {
  type        = string
  description = "AWS region (single-region multi-AZ topology)."
}

variable "run_id" {
  type        = string
  description = "Stable id for this run; tags every resource silt:awstest=<run_id> for teardown-by-tag."
}

variable "vpc_cidr" {
  type    = string
  default = "10.20.0.0/16"
}

variable "public_subnets" {
  type        = map(string)
  description = "AZ slot (0/1/2 as a string key) -> public subnet CIDR, from topology.py."
}

variable "nat_subnet" {
  type        = string
  description = "Private (natted) subnet CIDR, in AZ slot 0."
}

variable "nodes" {
  description = "name -> {role, ip, az, az_slot, subnet_cidr, region, argv} (non-natgw), from topology.py."
  type = map(object({
    role        = string
    ip          = string
    az          = string
    az_slot     = number
    subnet_cidr = string
    region      = string
    argv        = string
  }))
}

variable "swarm_port" {
  type    = number
  default = 4001
}
variable "relay_port" {
  type    = number
  default = 4002
}
variable "registry_port" {
  type    = number
  default = 8443
}

variable "instance_type" {
  type    = string
  default = "t3.small"
}

variable "boot_disk_gb" {
  type    = number
  default = 20
}

variable "ttl_minutes" {
  type        = number
  default     = 180
  description = "Hard self-destruct: user-data runs `shutdown -h +ttl_minutes`; instances terminate on shutdown."
}

variable "silt_binary_path" {
  type        = string
  description = "Local path to the linux/amd64 silt binary; uploaded to the run's S3 bucket."
}

variable "all_on_demand" {
  type        = bool
  default     = false
  description = "true => every node on-demand (a CERT run — Spot preemption is fatal to a cert). false => Spot for non-core."
}

variable "core_on_demand" {
  type        = bool
  default     = true
  description = "Keep the core roles (validator/registry/storage) on-demand even when all_on_demand=false."
}
