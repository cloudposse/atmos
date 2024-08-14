resource "google_compute_network" "network_1" {
  project                 = var.project_id_1
  name                    = "${module.this.id}-${var.region_1}"
  auto_create_subnetworks = var.auto_create_subnetworks

  provider = google.provider_1
}

resource "google_compute_network" "network_2" {
  project                 = var.project_id_2
  name                    = "${module.this.id}-${var.region_2}"
  auto_create_subnetworks = var.auto_create_subnetworks

  provider = google.provider_2
}

resource "google_compute_subnetwork" "subnets_1" {
  for_each = { for key, val in var.subnets : key => val }

  name          = "${module.this.id}-${var.region_1}-${each.value.name}"
  project       = var.project_id_1
  ip_cidr_range = each.value["ip_cidr_range"]
  region        = var.region_1
  network       = google_compute_network.network_1.id

  dynamic "secondary_ip_range" {
    for_each = each.value["secondary_ip_ranges"]
    content {
      range_name    = secondary_ip_range.value["range_name"]
      ip_cidr_range = secondary_ip_range.value["ip_cidr_range"]
    }
  }

  provider = google.provider_1
}

resource "google_compute_subnetwork" "subnets_2" {
  for_each = { for key, val in var.subnets : key => val }

  name          = "${module.this.id}-${var.region_2}-${each.value.name}"
  project       = var.project_id_2
  ip_cidr_range = each.value["ip_cidr_range"]
  region        = var.region_2
  network       = google_compute_network.network_2.id

  dynamic "secondary_ip_range" {
    for_each = each.value["secondary_ip_ranges"]
    content {
      range_name    = secondary_ip_range.value["range_name"]
      ip_cidr_range = secondary_ip_range.value["ip_cidr_range"]
    }
  }

  provider = google.provider_2
}
