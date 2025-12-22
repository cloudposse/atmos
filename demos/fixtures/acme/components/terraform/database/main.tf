locals {
  enabled = module.this.enabled

  partition = one(data.aws_partition.current[*].partition)

  deployed_cluster_identifier = local.enabled ? coalesce(one(aws_rds_cluster.primary[*].id), one(aws_rds_cluster.secondary[*].id)) : ""
  db_subnet_group_name        = one(aws_db_subnet_group.default[*].name)
  instance_class              = var.serverlessv2_scaling_configuration != null ? "db.serverless" : var.instance_type

  cluster_instance_count   = local.enabled ? var.cluster_size : 0
  is_regional_cluster      = var.cluster_type == "regional"
  is_serverless            = var.engine_mode == "serverless"
  is_serverless_v2         = var.instance_type == "db.serverless" && (contains(["aurora-postgresql", "aurora-mysql"], var.engine)) && var.engine_mode == "provisioned"
  enable_http_endpoint     = var.enable_http_endpoint && (local.is_serverless || local.is_serverless_v2)
  ignore_admin_credentials = var.replication_source_identifier != "" || var.snapshot_identifier != null
  reserved_instance_engine = var.engine
  use_reserved_instances   = var.use_reserved_instances && !local.is_serverless
}

data "aws_partition" "current" {
  count = local.enabled ? 1 : 0
}

# TODO: Use cloudposse/security-group module
resource "aws_security_group" "default" {
  count       = local.enabled ? 1 : 0
  name        = module.this.id
  description = "Allow inbound traffic from Security Groups and CIDRs"
  vpc_id      = var.vpc_id
  tags        = module.this.tags
}

resource "aws_security_group_rule" "ingress_security_groups" {
  count                    = local.enabled ? length(var.security_groups) : 0
  description              = "Allow inbound traffic from existing security groups"
  type                     = "ingress"
  from_port                = var.db_port
  to_port                  = var.db_port
  protocol                 = "tcp"
  source_security_group_id = var.security_groups[count.index]
  security_group_id        = join("", aws_security_group.default[*].id)
}

resource "aws_security_group_rule" "traffic_inside_security_group" {
  count             = local.enabled && var.intra_security_group_traffic_enabled ? 1 : 0
  description       = "Allow traffic between members of the database security group"
  type              = "ingress"
  from_port         = var.db_port
  to_port           = var.db_port
  protocol          = "tcp"
  self              = true
  security_group_id = join("", aws_security_group.default[*].id)
}

resource "aws_security_group_rule" "ingress_cidr_blocks" {
  count             = local.enabled && length(var.allowed_cidr_blocks) > 0 ? 1 : 0
  description       = "Allow inbound traffic from existing CIDR blocks"
  type              = "ingress"
  from_port         = var.db_port
  to_port           = var.db_port
  protocol          = "tcp"
  cidr_blocks       = var.allowed_cidr_blocks
  security_group_id = join("", aws_security_group.default[*].id)
}

resource "aws_security_group_rule" "ingress_ipv6_cidr_blocks" {
  count             = local.enabled && length(var.allowed_ipv6_cidr_blocks) > 0 ? 1 : 0
  description       = "Allow inbound traffic from existing CIDR blocks"
  type              = "ingress"
  from_port         = var.db_port
  to_port           = var.db_port
  protocol          = "tcp"
  ipv6_cidr_blocks  = var.allowed_ipv6_cidr_blocks
  security_group_id = join("", aws_security_group.default[*].id)
}

resource "aws_security_group_rule" "egress" {
  count             = local.enabled && var.egress_enabled ? 1 : 0
  description       = "Allow outbound traffic"
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = join("", aws_security_group.default[*].id)
}

resource "aws_security_group_rule" "egress_ipv6" {
  count             = local.enabled && var.egress_enabled ? 1 : 0
  description       = "Allow outbound ipv6 traffic"
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  ipv6_cidr_blocks  = ["::/0"]
  security_group_id = join("", aws_security_group.default[*].id)
}

data "aws_rds_reserved_instance_offering" "default" {
  count               = local.use_reserved_instances ? 1 : 0
  db_instance_class   = var.instance_type
  duration            = var.rds_ri_duration
  multi_az            = startswith(local.reserved_instance_engine, "aurora") ? false : local.cluster_instance_count > 1 # Aurora options never available for multi AZ for Reserved Instances. Single Reserved Instances rates still apply. https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/USER_WorkingWithReservedDBInstances.html
  offering_type       = var.rds_ri_offering_type
  product_description = local.reserved_instance_engine
}

# Note: I'm not sure what will happen when the db reservation expires, and this is not easy to test.
# It will either be recreated or will require manual intervention to recreate.
resource "aws_rds_reserved_instance" "default" {
  count = local.use_reserved_instances ? 1 : 0

  offering_id    = data.aws_rds_reserved_instance_offering.default[0].id
  instance_count = local.cluster_instance_count
  reservation_id = var.rds_ri_reservation_id

  lifecycle {
    # Once created, we want to avoid any case of accidentally re-creating.
    prevent_destroy = true
  }
}

# The name "primary" is poorly chosen. We actually mean standalone or regional.
# The primary cluster of a global database is actually created with the "secondary" cluster resource below.
resource "aws_rds_cluster" "primary" {
  count              = local.enabled && local.is_regional_cluster ? 1 : 0
  cluster_identifier = var.cluster_identifier == "" ? module.this.id : var.cluster_identifier
  database_name      = var.db_name
  # manage_master_user_password must be `null` or `true`. If it is `false`, and `master_password` is not `null`, a conflict occurs.
  manage_master_user_password           = var.manage_admin_user_password ? var.manage_admin_user_password : null
  master_user_secret_kms_key_id         = var.admin_user_secret_kms_key_id
  master_username                       = local.ignore_admin_credentials ? null : var.admin_user
  master_password                       = local.ignore_admin_credentials || var.manage_admin_user_password ? null : var.admin_password
  backup_retention_period               = var.retention_period
  preferred_backup_window               = var.backup_window
  copy_tags_to_snapshot                 = var.copy_tags_to_snapshot
  final_snapshot_identifier             = var.cluster_identifier == "" ? lower(module.this.id) : lower(var.cluster_identifier)
  skip_final_snapshot                   = var.skip_final_snapshot
  apply_immediately                     = var.apply_immediately
  db_cluster_instance_class             = local.is_serverless ? null : var.db_cluster_instance_class
  storage_encrypted                     = local.is_serverless ? null : var.storage_encrypted
  storage_type                          = var.storage_type
  iops                                  = var.iops
  allocated_storage                     = var.allocated_storage
  kms_key_id                            = var.kms_key_arn
  source_region                         = var.source_region
  snapshot_identifier                   = var.snapshot_identifier
  vpc_security_group_ids                = compact(flatten([join("", aws_security_group.default[*].id), var.vpc_security_group_ids]))
  preferred_maintenance_window          = var.maintenance_window
  network_type                          = var.network_type
  db_subnet_group_name                  = join("", aws_db_subnet_group.default[*].name)
  db_cluster_parameter_group_name       = join("", aws_rds_cluster_parameter_group.default[*].name)
  iam_database_authentication_enabled   = var.iam_database_authentication_enabled
  tags                                  = module.this.tags
  engine                                = var.engine
  engine_version                        = var.engine_version
  allow_major_version_upgrade           = var.allow_major_version_upgrade
  db_instance_parameter_group_name      = var.allow_major_version_upgrade ? join("", aws_db_parameter_group.default[*].name) : null
  engine_mode                           = var.engine_mode
  iam_roles                             = var.iam_roles
  backtrack_window                      = var.backtrack_window
  enable_http_endpoint                  = local.enable_http_endpoint
  port                                  = var.db_port
  enable_global_write_forwarding        = var.enable_global_write_forwarding
  enable_local_write_forwarding         = var.enable_local_write_forwarding
  performance_insights_enabled          = var.performance_insights_enabled
  performance_insights_kms_key_id       = var.performance_insights_kms_key_id
  performance_insights_retention_period = var.performance_insights_retention_period
  database_insights_mode                = var.database_insights_mode

  depends_on = [
    aws_db_subnet_group.default,
    aws_rds_cluster_parameter_group.default,
    aws_security_group.default,
  ]

  dynamic "s3_import" {
    for_each = var.s3_import[*]
    content {
      bucket_name           = lookup(s3_import.value, "bucket_name", null)
      bucket_prefix         = lookup(s3_import.value, "bucket_prefix", null)
      ingestion_role        = lookup(s3_import.value, "ingestion_role", null)
      source_engine         = lookup(s3_import.value, "source_engine", null)
      source_engine_version = lookup(s3_import.value, "source_engine_version", null)
    }
  }

  dynamic "scaling_configuration" {
    for_each = var.scaling_configuration
    content {
      auto_pause               = lookup(scaling_configuration.value, "auto_pause", null)
      max_capacity             = lookup(scaling_configuration.value, "max_capacity", null)
      min_capacity             = lookup(scaling_configuration.value, "min_capacity", null)
      seconds_until_auto_pause = lookup(scaling_configuration.value, "seconds_until_auto_pause", null)
      timeout_action           = lookup(scaling_configuration.value, "timeout_action", null)
    }
  }

  dynamic "serverlessv2_scaling_configuration" {
    for_each = var.serverlessv2_scaling_configuration[*]
    content {
      max_capacity             = serverlessv2_scaling_configuration.value.max_capacity
      min_capacity             = serverlessv2_scaling_configuration.value.min_capacity
      seconds_until_auto_pause = serverlessv2_scaling_configuration.value.seconds_until_auto_pause
    }
  }

  dynamic "timeouts" {
    for_each = var.timeouts_configuration
    content {
      create = lookup(timeouts.value, "create", "120m")
      update = lookup(timeouts.value, "update", "120m")
      delete = lookup(timeouts.value, "delete", "120m")
    }
  }

  dynamic "restore_to_point_in_time" {
    for_each = var.restore_to_point_in_time
    content {
      source_cluster_identifier = restore_to_point_in_time.value.source_cluster_identifier
      restore_type              = restore_to_point_in_time.value.restore_type
      # use_latest_restorable_time and restore_to_time are mutually exclusive.
      # If restore_to_time is given, then we ignore use_latest_restorable_time
      use_latest_restorable_time = restore_to_point_in_time.value.restore_to_time != null ? null : restore_to_point_in_time.value.use_latest_restorable_time
      restore_to_time            = restore_to_point_in_time.value.restore_to_time
    }
  }

  enabled_cloudwatch_logs_exports = var.enabled_cloudwatch_logs_exports
  deletion_protection             = var.deletion_protection
  replication_source_identifier   = var.replication_source_identifier
}

# https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/rds_cluster#replication_source_identifier
resource "aws_rds_cluster" "secondary" {
  count              = local.enabled && !local.is_regional_cluster ? 1 : 0
  cluster_identifier = var.cluster_identifier == "" ? module.this.id : var.cluster_identifier
  database_name      = var.db_name
  # manage_master_user_password must be `null` or `true`. If it is `false`, and `master_password` is not `null`, a conflict occurs.
  manage_master_user_password         = var.manage_admin_user_password ? var.manage_admin_user_password : null
  master_user_secret_kms_key_id       = var.admin_user_secret_kms_key_id
  master_username                     = local.ignore_admin_credentials ? null : var.admin_user
  master_password                     = local.ignore_admin_credentials || var.manage_admin_user_password ? null : var.admin_password
  backup_retention_period             = var.retention_period
  preferred_backup_window             = var.backup_window
  copy_tags_to_snapshot               = var.copy_tags_to_snapshot
  final_snapshot_identifier           = var.cluster_identifier == "" ? lower(module.this.id) : lower(var.cluster_identifier)
  skip_final_snapshot                 = var.skip_final_snapshot
  apply_immediately                   = var.apply_immediately
  db_cluster_instance_class           = local.is_serverless ? null : var.db_cluster_instance_class
  storage_encrypted                   = var.storage_encrypted
  storage_type                        = var.storage_type
  kms_key_id                          = var.kms_key_arn
  source_region                       = var.source_region
  snapshot_identifier                 = var.snapshot_identifier
  vpc_security_group_ids              = compact(flatten([join("", aws_security_group.default[*].id), var.vpc_security_group_ids]))
  preferred_maintenance_window        = var.maintenance_window
  network_type                        = var.network_type
  db_subnet_group_name                = join("", aws_db_subnet_group.default[*].name)
  db_cluster_parameter_group_name     = join("", aws_rds_cluster_parameter_group.default[*].name)
  iam_database_authentication_enabled = var.iam_database_authentication_enabled
  tags                                = module.this.tags
  engine                              = var.engine
  engine_version                      = var.engine_version
  allow_major_version_upgrade         = var.allow_major_version_upgrade
  engine_mode                         = var.engine_mode
  iam_roles                           = var.iam_roles
  backtrack_window                    = var.backtrack_window
  enable_http_endpoint                = local.enable_http_endpoint
  port                                = var.db_port
  enable_global_write_forwarding      = var.enable_global_write_forwarding
  enable_local_write_forwarding       = var.enable_local_write_forwarding
  database_insights_mode              = var.database_insights_mode

  depends_on = [
    aws_db_subnet_group.default,
    aws_db_parameter_group.default,
    aws_rds_cluster_parameter_group.default,
    aws_security_group.default,
  ]

  dynamic "scaling_configuration" {
    for_each = var.scaling_configuration
    content {
      auto_pause               = lookup(scaling_configuration.value, "auto_pause", null)
      max_capacity             = lookup(scaling_configuration.value, "max_capacity", null)
      min_capacity             = lookup(scaling_configuration.value, "min_capacity", null)
      seconds_until_auto_pause = lookup(scaling_configuration.value, "seconds_until_auto_pause", null)
      timeout_action           = lookup(scaling_configuration.value, "timeout_action", null)
    }
  }

  dynamic "serverlessv2_scaling_configuration" {
    for_each = var.serverlessv2_scaling_configuration[*]
    content {
      max_capacity = serverlessv2_scaling_configuration.value.max_capacity
      min_capacity = serverlessv2_scaling_configuration.value.min_capacity
    }
  }

  dynamic "timeouts" {
    for_each = var.timeouts_configuration
    content {
      create = lookup(timeouts.value, "create", "120m")
      update = lookup(timeouts.value, "update", "120m")
      delete = lookup(timeouts.value, "delete", "120m")
    }
  }

  enabled_cloudwatch_logs_exports = var.enabled_cloudwatch_logs_exports
  deletion_protection             = var.deletion_protection

  global_cluster_identifier = var.global_cluster_identifier

  # https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/rds_cluster#replication_source_identifier
  # ARN of a source DB cluster or DB instance if this DB cluster is to be created as a Read Replica.
  # If DB Cluster is part of a Global Cluster, use the lifecycle configuration block ignore_changes argument
  # to prevent Terraform from showing differences for this argument instead of configuring this value.

  lifecycle {
    ignore_changes = [
      replication_source_identifier, # will be set/managed by Global Cluster
      snapshot_identifier,           # if created from a snapshot, will be non-null at creation, but null afterwards
    ]
  }
}

resource "random_pet" "instance" {
  count  = local.enabled ? 1 : 0
  prefix = var.cluster_identifier == "" ? module.this.id : var.cluster_identifier
  keepers = {
    cluster_family = var.cluster_family
    instance_class = var.serverlessv2_scaling_configuration != null ? "db.serverless" : var.instance_type
  }
}

module "rds_identifier" {
  count = local.enabled ? 1 : 0

  source  = "cloudposse/label/null"
  version = "0.25.0"

  name = random_pet.instance[0].id
  # Max length of RDS identifier is 63 characters, but in `aws_rds_cluster_instance`
  # we append the instance index to the identifier
  # Setting the limit to 60 allow to use up to 99 instances, when only 16 is allowed
  # (1 writer + 15 readers)
  # https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Aurora.Replication.html
  id_length_limit = 60
}

resource "aws_rds_cluster_instance" "default" {
  count                                 = local.cluster_instance_count
  identifier                            = "${module.rds_identifier[0].id}-${count.index + 1}"
  cluster_identifier                    = local.deployed_cluster_identifier
  instance_class                        = local.instance_class
  db_subnet_group_name                  = local.db_subnet_group_name
  db_parameter_group_name               = join("", aws_db_parameter_group.default[*].name)
  publicly_accessible                   = var.publicly_accessible
  tags                                  = module.this.tags
  engine                                = var.engine
  engine_version                        = var.engine_version
  auto_minor_version_upgrade            = var.auto_minor_version_upgrade
  monitoring_interval                   = var.rds_monitoring_interval
  monitoring_role_arn                   = var.enhanced_monitoring_role_enabled ? join("", aws_iam_role.enhanced_monitoring[*].arn) : var.rds_monitoring_role_arn
  performance_insights_enabled          = var.performance_insights_enabled
  performance_insights_kms_key_id       = var.performance_insights_kms_key_id
  performance_insights_retention_period = var.performance_insights_retention_period
  availability_zone                     = var.instance_availability_zone
  apply_immediately                     = var.apply_immediately
  preferred_maintenance_window          = var.maintenance_window
  copy_tags_to_snapshot                 = var.copy_tags_to_snapshot
  ca_cert_identifier                    = var.ca_cert_identifier
  promotion_tier                        = var.promotion_tier

  dynamic "timeouts" {
    for_each = var.timeouts_configuration
    content {
      create = lookup(timeouts.value, "create", "120m")
      update = lookup(timeouts.value, "update", "120m")
      delete = lookup(timeouts.value, "delete", "120m")
    }
  }

  depends_on = [
    aws_db_subnet_group.default,
    aws_db_parameter_group.default,
    aws_iam_role.enhanced_monitoring,
    aws_rds_cluster.secondary,
    aws_rds_cluster_parameter_group.default,
  ]

  lifecycle {
    ignore_changes        = [engine_version]
    create_before_destroy = true
  }
}

resource "aws_db_subnet_group" "default" {
  count       = local.enabled ? 1 : 0
  name        = try(length(var.subnet_group_name), 0) == 0 ? module.this.id : var.subnet_group_name
  description = "Allowed subnets for DB cluster instances"
  subnet_ids  = var.subnets
  tags        = module.this.tags
}

resource "aws_rds_cluster_parameter_group" "default" {
  count = local.enabled ? 1 : 0

  name_prefix = var.parameter_group_name_prefix_enabled ? "${coalesce(var.rds_cluster_parameter_group_name, module.this.id)}${module.this.delimiter}" : null
  name        = !var.parameter_group_name_prefix_enabled ? coalesce(var.rds_cluster_parameter_group_name, module.this.id) : null

  description = "DB cluster parameter group"
  family      = var.cluster_family

  dynamic "parameter" {
    for_each = var.cluster_parameters
    content {
      apply_method = lookup(parameter.value, "apply_method", null)
      name         = parameter.value.name
      value        = parameter.value.value
    }
  }

  tags = module.this.tags

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_db_parameter_group" "default" {
  count = local.enabled ? 1 : 0

  name_prefix = var.parameter_group_name_prefix_enabled ? "${coalesce(var.db_parameter_group_name, module.this.id)}${module.this.delimiter}" : null
  name        = !var.parameter_group_name_prefix_enabled ? coalesce(var.db_parameter_group_name, module.this.id) : null

  description = "DB instance parameter group"
  family      = var.cluster_family

  dynamic "parameter" {
    for_each = var.instance_parameters
    content {
      apply_method = lookup(parameter.value, "apply_method", null)
      name         = parameter.value.name
      value        = parameter.value.value
    }
  }

  tags = module.this.tags

  lifecycle {
    create_before_destroy = true
  }
}

locals {
  cluster_dns_name_default = "master.${module.this.name}"
  cluster_dns_name         = var.cluster_dns_name != "" ? var.cluster_dns_name : local.cluster_dns_name_default
  reader_dns_name_default  = "replicas.${module.this.name}"
  reader_dns_name          = var.reader_dns_name != "" ? var.reader_dns_name : local.reader_dns_name_default
}

module "dns_master" {
  source  = "cloudposse/route53-cluster-hostname/aws"
  version = "0.13.0"

  enabled  = local.enabled && length(var.zone_id) > 0
  dns_name = local.cluster_dns_name
  zone_id  = try(var.zone_id[0], tostring(var.zone_id), "")
  records  = coalescelist(aws_rds_cluster.primary[*].endpoint, aws_rds_cluster.secondary[*].endpoint, [""])

  context = module.this.context
}

module "dns_replicas" {
  source  = "cloudposse/route53-cluster-hostname/aws"
  version = "0.13.0"

  enabled  = local.enabled && length(var.zone_id) > 0 && !local.is_serverless && local.cluster_instance_count > 0
  dns_name = local.reader_dns_name
  zone_id  = try(var.zone_id[0], tostring(var.zone_id), "")
  records  = coalescelist(aws_rds_cluster.primary[*].reader_endpoint, aws_rds_cluster.secondary[*].reader_endpoint, [""])

  context = module.this.context
}

resource "aws_appautoscaling_target" "replicas" {
  count              = local.enabled && var.autoscaling_enabled ? 1 : 0
  service_namespace  = "rds"
  scalable_dimension = "rds:cluster:ReadReplicaCount"
  resource_id        = "cluster:${local.deployed_cluster_identifier}"
  min_capacity       = var.autoscaling_min_capacity
  max_capacity       = var.autoscaling_max_capacity
}

resource "aws_appautoscaling_policy" "replicas" {
  count              = local.enabled && var.autoscaling_enabled ? 1 : 0
  name               = module.this.id
  service_namespace  = join("", aws_appautoscaling_target.replicas[*].service_namespace)
  scalable_dimension = join("", aws_appautoscaling_target.replicas[*].scalable_dimension)
  resource_id        = join("", aws_appautoscaling_target.replicas[*].resource_id)
  policy_type        = var.autoscaling_policy_type

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = var.autoscaling_target_metrics
    }

    disable_scale_in   = false
    target_value       = var.autoscaling_target_value
    scale_in_cooldown  = var.autoscaling_scale_in_cooldown
    scale_out_cooldown = var.autoscaling_scale_out_cooldown
  }
}

resource "aws_rds_cluster_activity_stream" "primary" {
  count = local.enabled && var.activity_stream_enabled ? 1 : 0

  resource_arn = join("", aws_rds_cluster.primary[*].arn)
  mode         = var.activity_stream_mode
  kms_key_id   = var.activity_stream_kms_key_id
}
