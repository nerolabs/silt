# The PERSISTENT network for cloudtest runs (harness-hardening item c,
# 2026-08-19 audit): the VPC, subnets, firewalls, and Cloud NAT router are
# identical every run yet cost several minutes to create and destroy each time.
# This root module owns them once, in its own state; a run launched with
# PERSIST_NET=1 data-sources them instead of creating its own (see ../main.tf
# `persistent_network`). Everything here is REGION-CANONICAL: the subnets use
# topology.py's canonical region→octet mapping (default 20; europe-west1 21;
# us-east1 22; 30 reserved for the NAT subnet), which is a function of the
# region alone — safe for every topology subset (SMOKE through the full
# SYBILS+MATURING+ECONOMY sheet).
#
#   terraform -chdir=terraform/network apply  -var project_id=… [-var default_region=…]
#   (or: ./cloudtest.sh net-up / net-down)
#
# Nothing here carries a run_id, and a `cloudtest.sh destroy` never touches it.
# Cost while idle: $0 (VPC/subnets/firewalls are free; Cloud NAT bills only for
# egress processed, and idle runs none).

variable "project_id" { type = string }

variable "default_region" {
  type    = string
  default = "us-west1"
}

# Canonical public subnets. Keys are regions, values octets — MUST mirror
# topology.py's canonical assignment. Extend here (and there) when a run needs
# a region beyond the standard trio.
variable "region_octets" {
  type = map(number)
  default = {
    "us-west1"     = 20
    "europe-west1" = 21
    "us-east1"     = 22
  }
}

variable "nat_cidr" {
  type    = string
  default = "10.30.0.0/24"
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

provider "google" {
  project = var.project_id
}

resource "google_compute_network" "silt" {
  name                    = "silt-persist"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "public" {
  for_each      = var.region_octets
  name          = "silt-persist-public-${each.key}"
  ip_cidr_range = "10.${each.value}.0.0/24"
  region        = each.key
  network       = google_compute_network.silt.id
}

resource "google_compute_subnetwork" "nat" {
  name          = "silt-persist-nat"
  ip_cidr_range = var.nat_cidr
  region        = var.default_region
  network       = google_compute_network.silt.id
}

# Firewalls — same three rules as the per-run network (../main.tf), minus run_id
# names. All-internal allow, IAP-only SSH, public swarm/relay/registry ports.
resource "google_compute_firewall" "internal" {
  name          = "silt-persist-internal"
  network       = google_compute_network.silt.id
  source_ranges = concat([for o in values(var.region_octets) : "10.${o}.0.0/24"], [var.nat_cidr])
  allow { protocol = "tcp" }
  allow { protocol = "udp" }
  allow { protocol = "icmp" }
}

resource "google_compute_firewall" "iap_ssh" {
  name          = "silt-persist-iap-ssh"
  network       = google_compute_network.silt.id
  source_ranges = ["35.235.240.0/20"]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

resource "google_compute_firewall" "public_swarm" {
  name          = "silt-persist-swarm"
  network       = google_compute_network.silt.id
  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["public-node"]
  allow {
    protocol = "tcp"
    ports    = [tostring(var.swarm_port), tostring(var.relay_port), tostring(var.registry_port)]
  }
}

# Cloud NAT for the no-external-IP nodes (island + ECONOMY noip stores): managed,
# no instance, bills only processed egress. The per-run module skips creating its
# own when persistent_network is on.
resource "google_compute_router" "noip" {
  name    = "silt-persist-router"
  region  = var.default_region
  network = google_compute_network.silt.id
}

resource "google_compute_router_nat" "noip" {
  name                               = "silt-persist-nat-gw"
  router                             = google_compute_router.noip.name
  region                             = var.default_region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

output "network" { value = google_compute_network.silt.name }
output "subnets" { value = { for r, s in google_compute_subnetwork.public : r => s.name } }
