variable "region" {
  type        = string
  description = "AWS Region."
}

variable "autoscale_write_target" {
  type        = number
  default     = 50
  description = "The target value (in %) for DynamoDB write autoscaling"
}

variable "autoscale_read_target" {
  type        = number
  default     = 50
  description = "The target value (in %) for DynamoDB read autoscaling"
}

variable "autoscale_min_read_capacity" {
  type        = number
  default     = 5
  description = "DynamoDB autoscaling min read capacity"
}

variable "autoscale_max_read_capacity" {
  type        = number
  default     = 20
  description = "DynamoDB autoscaling max read capacity"
}

variable "autoscale_min_write_capacity" {
  type        = number
  default     = 5
  description = "DynamoDB autoscaling min write capacity"
}

variable "autoscale_max_write_capacity" {
  type        = number
  default     = 20
  description = "DynamoDB autoscaling max write capacity"
}

variable "billing_mode" {
  type        = string
  default     = "PROVISIONED"
  description = "DynamoDB Billing mode. Can be PROVISIONED or PAY_PER_REQUEST"
}

variable "streams_enabled" {
  type        = bool
  default     = false
  description = "Enable DynamoDB streams"
}

variable "stream_view_type" {
  type        = string
  default     = ""
  description = "When an item in the table is modified, what information is written to the stream"
}

variable "encryption_enabled" {
  type        = bool
  default     = true
  description = "Enable DynamoDB server-side encryption"
}

variable "server_side_encryption_kms_key_arn" {
  type        = string
  default     = null
  description = "The ARN of the CMK that should be used for the AWS KMS encryption. This attribute should only be specified if the key is different from the default DynamoDB CMK, alias/aws/dynamodb."
}

variable "point_in_time_recovery_enabled" {
  type        = bool
  default     = true
  description = "Enable DynamoDB point in time recovery"
}

variable "hash_key" {
  type        = string
  description = "DynamoDB table Hash Key"
}

variable "hash_key_type" {
  type        = string
  default     = "S"
  description = "Hash Key type, which must be a scalar type: `S`, `N`, or `B` for String, Number or Binary data, respectively."
}

variable "range_key" {
  type        = string
  default     = ""
  description = "DynamoDB table Range Key"
}

variable "range_key_type" {
  type        = string
  default     = "S"
  description = "Range Key type, which must be a scalar type: `S`, `N`, or `B` for String, Number or Binary data, respectively."
}

variable "ttl_attribute" {
  type        = string
  default     = ""
  description = "DynamoDB table TTL attribute"
}

variable "ttl_enabled" {
  type        = bool
  default     = false
  description = "Set to false to disable DynamoDB table TTL"
}

variable "autoscaler_enabled" {
  type        = bool
  default     = false
  description = "Flag to enable/disable DynamoDB autoscaling"
}

variable "autoscaler_attributes" {
  type        = list(string)
  default     = []
  description = "Additional attributes for the autoscaler module"
}

variable "autoscaler_tags" {
  type        = map(string)
  default     = {}
  description = "Additional resource tags for the autoscaler module"
}

variable "table_name" {
  type        = string
  default     = null
  description = "Table name. If provided, the bucket will be created with this name instead of generating the name from the context"
}

variable "dynamodb_attributes" {
  type = list(object({
    name = string
    type = string
  }))
  default     = []
  description = "Additional DynamoDB attributes in the form of a list of mapped values"
}

variable "global_secondary_index_map" {
  type = list(object({
    hash_key           = string
    name               = string
    non_key_attributes = list(string)
    projection_type    = string
    range_key          = string
    read_capacity      = number
    write_capacity     = number
  }))
  default     = []
  description = "Additional global secondary indexes in the form of a list of mapped values"
}

variable "local_secondary_index_map" {
  type = list(object({
    name               = string
    non_key_attributes = list(string)
    projection_type    = string
    range_key          = string
  }))
  default     = []
  description = "Additional local secondary indexes in the form of a list of mapped values"
}

variable "replicas" {
  type        = list(string)
  default     = []
  description = "List of regions to create a replica table in"
}

variable "deletion_protection_enabled" {
  type        = bool
  default     = false
  description = "Enable/disable DynamoDB table deletion protection"
}

variable "import_table" {
  type = object({
    # Valid values are GZIP, ZSTD and NONE
    input_compression_type = optional(string, null)
    # Valid values are CSV, DYNAMODB_JSON, and ION.
    input_format = string
    input_format_options = optional(object({
      csv = object({
        delimiter   = string
        header_list = list(string)
      })
    }), null)
    s3_bucket_source = object({
      bucket       = string
      bucket_owner = optional(string)
      key_prefix   = optional(string)
    })
  })
  default     = null
  description = "Import Amazon S3 data into a new table."
}
