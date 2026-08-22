# The ephemeral field-test network. Everything here is created on `apply` and
# removed on `destroy`. Instances are SPOT (preemptible) and self-destruct after
# var.ttl_minutes as a cost backstop — see the triple cost-guard in the README.

locals {
  # Split the node table by network role.
  public_nodes = { for k, v in var.nodes : k => v if v.role != "natted" && v.role != "natgw" && v.role != "island" && !v.internal_only }
  nat_nodes    = { for k, v in var.nodes : k => v if v.role == "natted" }
  natgw_nodes  = { for k, v in var.nodes : k => v if v.role == "natgw" }
  # The equivocation island: a contained consensus universe (own anchors, own
  # genesis; nothing in the main swarm names it). NO external IP → zero
  # IN_USE_ADDRESSES quota (the real constraint); egress for the GCS binary pull
  # goes through Cloud NAT below. Design: docs/thinking/2026-08-20-equivocation-island-design.md.
  island_nodes = { for k, v in var.nodes : k => v if v.role == "island" }
  # Main-swarm nodes with NO external IP (the ECONOMY killable stores): full
  # swarm members on the public subnet — dialable on internal IPs like every
  # node (the swarm advertises internal IPs) — but zero IN_USE_ADDRESSES quota.
  # Egress (GCS binary pull) via the same Cloud NAT as the island; reached over
  # IAP like everything else. docs/thinking/2026-08-20-economy-premise-killable-pool.md.
  noip_nodes = { for k, v in var.nodes : k => v if v.role != "island" && v.internal_only }
  labels     = { cloudtest = var.run_id }
}

# ── Network ────────────────────────────────────────────────────────────────────
# Two modes (harness-hardening item c): the default creates an ephemeral per-run
# network exactly as before; `persistent_network = true` (PERSIST_NET=1) instead
# data-sources the long-lived network the terraform/network module owns —
# subnets/firewalls/Cloud-NAT already exist, saving their create+destroy minutes
# every run. The persistent subnets use topology.py's CANONICAL region octets,
# so every topology subset lands on the same CIDRs.
resource "google_compute_network" "silt" {
  count                   = var.persistent_network ? 0 : 1
  name                    = "silt-ft-${var.run_id}"
  auto_create_subnetworks = false
}

data "google_compute_network" "persist" {
  count = var.persistent_network ? 1 : 0
  name  = "silt-persist"
}

# One public subnet PER region the topology uses — GCP subnets are regional, so a
# node in us-east1 must attach to a us-east1 subnet. topology.py emits region_cidrs.
resource "google_compute_subnetwork" "public" {
  for_each      = var.persistent_network ? {} : var.region_cidrs
  name          = "silt-ft-public-${each.key}-${var.run_id}"
  ip_cidr_range = each.value
  region        = each.key
  network       = google_compute_network.silt[0].id
}

data "google_compute_subnetwork" "persist_public" {
  for_each = var.persistent_network ? var.region_cidrs : {}
  name     = "silt-persist-public-${each.key}"
  region   = each.key
}

# The NAT subnet lives in the same region as the natgw. Its instances get NO
# external IP; their default route is the natgw instance, so they are genuinely
# un-dialable from the swarm and must reach it through the relay (or hole-punch).
resource "google_compute_subnetwork" "nat" {
  count         = var.persistent_network ? 0 : 1
  name          = "silt-ft-nat-${var.run_id}"
  ip_cidr_range = var.nat_cidr
  region        = var.default_region
  network       = google_compute_network.silt[0].id
}

data "google_compute_subnetwork" "persist_nat" {
  count  = var.persistent_network ? 1 : 0
  name   = "silt-persist-nat"
  region = var.default_region
}

locals {
  network_id    = var.persistent_network ? data.google_compute_network.persist[0].id : google_compute_network.silt[0].id
  subnet_ids    = var.persistent_network ? { for r, s in data.google_compute_subnetwork.persist_public : r => s.id } : { for r, s in google_compute_subnetwork.public : r => s.id }
  nat_subnet_id = var.persistent_network ? data.google_compute_subnetwork.persist_nat[0].id : google_compute_subnetwork.nat[0].id
}

# Route the NAT subnet's egress through the natgw instance (real NAT/conntrack).
resource "google_compute_route" "nat_egress" {
  count             = length(local.natgw_nodes) > 0 ? 1 : 0
  name              = "silt-ft-natroute-${var.run_id}"
  network           = local.network_id
  dest_range        = "0.0.0.0/0"
  next_hop_instance = google_compute_instance.natgw[one(keys(local.natgw_nodes))].self_link
  priority          = 900
  tags              = ["natted"]
}

# ── Firewall ───────────────────────────────────────────────────────────────────
# All traffic within the VPC is allowed (the swarm talks over internal IPs).
resource "google_compute_firewall" "internal" {
  count         = var.persistent_network ? 0 : 1
  name          = "silt-ft-internal-${var.run_id}"
  network       = local.network_id
  source_ranges = concat(values(var.region_cidrs), [var.nat_cidr])
  allow { protocol = "tcp" }
  allow { protocol = "udp" }
  allow { protocol = "icmp" }
}

# SSH from IAP only (35.235.240.0/20) — the orchestrator drives every node,
# including natted ones, over `gcloud compute ssh --tunnel-through-iap`.
resource "google_compute_firewall" "iap_ssh" {
  count         = var.persistent_network ? 0 : 1
  name          = "silt-ft-iap-ssh-${var.run_id}"
  network       = local.network_id
  source_ranges = ["35.235.240.0/20"]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

# Public swarm/relay/registry reachability for the public-subnet nodes only.
resource "google_compute_firewall" "public_swarm" {
  count         = var.persistent_network ? 0 : 1
  name          = "silt-ft-swarm-${var.run_id}"
  network       = local.network_id
  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["public-node"]
  allow {
    protocol = "tcp"
    ports    = [tostring(var.swarm_port), tostring(var.relay_port), tostring(var.registry_port)]
  }
}

# ── The binary, shipped via GCS ────────────────────────────────────────────────
resource "google_storage_bucket" "artifacts" {
  name                        = "silt-ft-${var.run_id}-${var.project_id}"
  location                    = var.default_region
  force_destroy               = true
  uniform_bucket_level_access = true
  labels                      = local.labels
}

resource "google_storage_bucket_object" "silt_binary" {
  name   = "silt-linux-amd64"
  bucket = google_storage_bucket.artifacts.name
  source = var.silt_binary_path
}

# Let the VMs' default compute service account read the bucket.
data "google_compute_default_service_account" "default" {}

resource "google_storage_bucket_iam_member" "vm_read" {
  bucket = google_storage_bucket.artifacts.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${data.google_compute_default_service_account.default.email}"
}

# ── Startup script (shared by all silt nodes) ──────────────────────────────────
locals {
  ssh_keys_meta = var.ssh_pubkey == "" ? {} : { "ssh-keys" = var.ssh_pubkey }
}

# ── NAT gateway instance ───────────────────────────────────────────────────────
resource "google_compute_instance" "natgw" {
  for_each     = local.natgw_nodes
  name         = "silt-ft-${each.key}-${var.run_id}"
  machine_type = var.machine_type
  zone         = each.value.zone
  labels       = local.labels
  tags         = ["natgw", "public-node"]

  can_ip_forward = true # required to route the NAT subnet's traffic

  boot_disk {
    initialize_params {
      image = var.image
      size  = var.boot_disk_gb
    }
  }
  network_interface {
    subnetwork = local.subnet_ids[each.value.region]
    network_ip = each.value.ip
    access_config {} # external IP so it can masquerade the NAT subnet out to the internet
  }
  scheduling {
    provisioning_model = "SPOT"
    preemptible        = true
    automatic_restart  = false
    # Orchestrator-independent hard cost backstop — GCP DELETEs the VM after the
    # TTL even if the destroy trap never runs (see the public-node scheduling note).
    instance_termination_action = "DELETE"
    max_run_duration {
      seconds = var.ttl_minutes * 60
    }
  }
  metadata = merge(local.ssh_keys_meta, {
    "startup-script" = templatefile("${path.module}/../provision/natgw-startup.sh", {
      nat_cidr    = var.nat_cidr
      ttl_minutes = var.ttl_minutes
    })
  })
}

# ── silt nodes on the public subnet ────────────────────────────────────────────
resource "google_compute_instance" "public" {
  for_each     = local.public_nodes
  name         = "silt-ft-${each.key}-${var.run_id}"
  machine_type = var.machine_type
  zone         = each.value.zone
  labels       = merge(local.labels, { role = each.value.role })
  tags         = ["public-node"]

  boot_disk {
    initialize_params {
      image = var.image
      size  = var.boot_disk_gb
    }
  }
  network_interface {
    subnetwork = local.subnet_ids[each.value.region]
    network_ip = each.value.ip
    access_config {} # external IP; the swarm still advertises the internal IP
  }
  # SPOT resilience (blind cloud finding): mid-run preemption of the consensus/
  # registry CORE cascaded 6 publish-dependent flows to "fail". With
  # var.core_on_demand set, the validator + registry nodes run STANDARD
  # (non-preemptible, auto-restart) so an RC-gate run isn't flaky; every other
  # role (storage/fetcher/relay/adversary/nat) stays SPOT for cost. All-SPOT
  # (cheap shakedowns) is core_on_demand=false.
  scheduling {
    provisioning_model = (var.all_on_demand || (var.core_on_demand && contains(["validator", "registry"], each.value.role))) ? "STANDARD" : "SPOT"
    preemptible        = !(var.all_on_demand || (var.core_on_demand && contains(["validator", "registry"], each.value.role)))
    automatic_restart  = false
    # Hard cost backstop that does NOT depend on the orchestrator: GCP itself
    # DELETES the VM after ttl_minutes, even if the destroy-on-EXIT trap never runs
    # (e.g. the orchestrator is SIGKILLed mid-run — which once leaked 6 on-demand
    # VMs). Stronger than the in-guest `shutdown -h +TTL`, which only halts the OS.
    instance_termination_action = "DELETE"
    max_run_duration {
      seconds = var.ttl_minutes * 60
    }
  }
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["https://www.googleapis.com/auth/devstorage.read_only"]
  }
  metadata = merge(local.ssh_keys_meta, {
    "silt-argv"       = each.value.argv
    "silt-binary-url" = "gs://${google_storage_bucket.artifacts.name}/${google_storage_bucket_object.silt_binary.name}"
    "node-name"       = each.key
    "startup-script" = templatefile("${path.module}/../provision/silt-startup.sh", {
      ttl_minutes = var.ttl_minutes
    })
  })
  depends_on = [google_storage_bucket_iam_member.vm_read]
}

# ── silt nodes on the NAT subnet (no external IP) ───────────────────────────────
resource "google_compute_instance" "natted" {
  for_each     = local.nat_nodes
  name         = "silt-ft-${each.key}-${var.run_id}"
  machine_type = var.machine_type
  zone         = each.value.zone
  labels       = merge(local.labels, { role = each.value.role })
  tags         = ["natted"]

  boot_disk {
    initialize_params {
      image = var.image
      size  = var.boot_disk_gb
    }
  }
  network_interface {
    subnetwork = local.nat_subnet_id
    network_ip = each.value.ip
    # NO access_config: no external IP → egress via the natgw, un-dialable inbound.
  }
  # SPOT resilience (blind cloud finding): mid-run preemption of the consensus/
  # registry CORE cascaded 6 publish-dependent flows to "fail". With
  # var.core_on_demand set, the validator + registry nodes run STANDARD
  # (non-preemptible, auto-restart) so an RC-gate run isn't flaky; every other
  # role (storage/fetcher/relay/adversary/nat) stays SPOT for cost. All-SPOT
  # (cheap shakedowns) is core_on_demand=false.
  scheduling {
    provisioning_model = (var.all_on_demand || (var.core_on_demand && contains(["validator", "registry"], each.value.role))) ? "STANDARD" : "SPOT"
    preemptible        = !(var.all_on_demand || (var.core_on_demand && contains(["validator", "registry"], each.value.role)))
    automatic_restart  = false
    # Hard cost backstop that does NOT depend on the orchestrator: GCP itself
    # DELETES the VM after ttl_minutes, even if the destroy-on-EXIT trap never runs
    # (e.g. the orchestrator is SIGKILLed mid-run — which once leaked 6 on-demand
    # VMs). Stronger than the in-guest `shutdown -h +TTL`, which only halts the OS.
    instance_termination_action = "DELETE"
    max_run_duration {
      seconds = var.ttl_minutes * 60
    }
  }
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["https://www.googleapis.com/auth/devstorage.read_only"]
  }
  metadata = merge(local.ssh_keys_meta, {
    "silt-argv"       = each.value.argv
    "silt-binary-url" = "gs://${google_storage_bucket.artifacts.name}/${google_storage_bucket_object.silt_binary.name}"
    "node-name"       = each.key
    "startup-script" = templatefile("${path.module}/../provision/silt-startup.sh", {
      ttl_minutes = var.ttl_minutes
    })
  })
  depends_on = [google_compute_instance.natgw, google_storage_bucket_iam_member.vm_read]
}

# ── Main-swarm silt nodes with NO external IP (the ECONOMY killable stores) ─────
# Identical to a public node — same subnet, same firewall tag, same startup, a
# full swarm member dialable on its internal IP — minus access_config, so it
# consumes zero IN_USE_ADDRESSES quota (ECONOMY=1 SYBILS=8 saturates every
# region's default 8). Egress via the Cloud NAT below; reached over IAP.
resource "google_compute_instance" "noip" {
  for_each     = local.noip_nodes
  name         = "silt-ft-${each.key}-${var.run_id}"
  machine_type = var.machine_type
  zone         = each.value.zone
  labels       = merge(local.labels, { role = each.value.role })
  tags         = ["public-node"]

  boot_disk {
    initialize_params {
      image = var.image
      size  = var.boot_disk_gb
    }
  }
  network_interface {
    subnetwork = local.subnet_ids[each.value.region]
    network_ip = each.value.ip
    # NO access_config: no external IP → zero quota. Egress via Cloud NAT.
  }
  scheduling {
    # Same policy as public nodes of this role (storage is SPOT unless
    # all_on_demand — a cert run protects content-holders, see variables.tf).
    provisioning_model          = var.all_on_demand ? "STANDARD" : "SPOT"
    preemptible                 = !var.all_on_demand
    automatic_restart           = false
    instance_termination_action = "DELETE"
    max_run_duration {
      seconds = var.ttl_minutes * 60
    }
  }
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["https://www.googleapis.com/auth/devstorage.read_only"]
  }
  metadata = merge(local.ssh_keys_meta, {
    "silt-argv"       = each.value.argv
    "silt-binary-url" = "gs://${google_storage_bucket.artifacts.name}/${google_storage_bucket_object.silt_binary.name}"
    "node-name"       = each.key
    "startup-script" = templatefile("${path.module}/../provision/silt-startup.sh", {
      ttl_minutes = var.ttl_minutes
    })
  })
  depends_on = [google_storage_bucket_iam_member.vm_read, google_compute_router_nat.island]
}

# ── The equivocation island: contained silt nodes with NO external IP ───────────
# A separate consensus universe for the one destructive drill (permanent-eviction
# equivocation). No access_config → no external IP → zero IN_USE_ADDRESSES quota.
# Cloud NAT (below) gives them outbound-only egress for the GCS binary pull; they
# stay un-dialable from the internet and, by their own anchor config, un-joined to
# the main swarm — so the drill's slash consumes only the island's fault tolerance.
resource "google_compute_instance" "island" {
  for_each     = local.island_nodes
  name         = "silt-ft-${each.key}-${var.run_id}"
  machine_type = var.machine_type
  zone         = each.value.zone
  labels       = merge(local.labels, { role = each.value.role })
  tags         = ["island"]

  boot_disk {
    initialize_params {
      image = var.image
      size  = var.boot_disk_gb
    }
  }
  network_interface {
    subnetwork = local.subnet_ids[each.value.region]
    network_ip = each.value.ip
    # NO access_config: no external IP → zero quota. Egress via Cloud NAT.
  }
  scheduling {
    # The island's whole job is the destructive drill; keep it cheap on SPOT. If
    # preempted, the drill GAPs "did not drive" (UNTESTED), never a false FAIL.
    provisioning_model          = var.all_on_demand ? "STANDARD" : "SPOT"
    preemptible                 = !var.all_on_demand
    automatic_restart           = false
    instance_termination_action = "DELETE"
    max_run_duration {
      seconds = var.ttl_minutes * 60
    }
  }
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["https://www.googleapis.com/auth/devstorage.read_only"]
  }
  metadata = merge(local.ssh_keys_meta, {
    "silt-argv"       = each.value.argv
    "silt-binary-url" = "gs://${google_storage_bucket.artifacts.name}/${google_storage_bucket_object.silt_binary.name}"
    "node-name"       = each.key
    "startup-script" = templatefile("${path.module}/../provision/silt-startup.sh", {
      ttl_minutes = var.ttl_minutes
    })
  })
  depends_on = [google_storage_bucket_iam_member.vm_read, google_compute_router_nat.island]
}

# Cloud NAT for outbound egress (GCS binary pull) of every no-external-IP node
# in the primary region — the island AND the ECONOMY noip stores. Only instances
# WITHOUT an external IP use it, so the public nodes are unaffected. Managed, no
# instance, ~free; created only when a no-IP node exists.
resource "google_compute_router" "island" {
  count   = (!var.persistent_network && (length(local.island_nodes) + length(local.noip_nodes)) > 0) ? 1 : 0
  name    = "silt-ft-island-router-${var.run_id}"
  region  = var.default_region
  network = local.network_id
}

resource "google_compute_router_nat" "island" {
  count                              = (!var.persistent_network && (length(local.island_nodes) + length(local.noip_nodes)) > 0) ? 1 : 0
  name                               = "silt-ft-island-nat-${var.run_id}"
  router                             = google_compute_router.island[0].name
  region                             = var.default_region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

# ── Optional budget backstop ───────────────────────────────────────────────────
resource "google_billing_budget" "cap" {
  count           = var.budget_amount_usd > 0 && var.billing_account != "" ? 1 : 0
  billing_account = var.billing_account
  display_name    = "silt-cloudtest-${var.run_id}"
  budget_filter {
    projects = ["projects/${var.project_id}"]
  }
  amount {
    specified_amount {
      currency_code = "USD"
      units         = tostring(var.budget_amount_usd)
    }
  }
  threshold_rules { threshold_percent = 0.5 }
  threshold_rules { threshold_percent = 0.9 }
  threshold_rules { threshold_percent = 1.0 }
}
