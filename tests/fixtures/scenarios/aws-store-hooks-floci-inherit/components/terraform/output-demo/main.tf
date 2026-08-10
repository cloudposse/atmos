terraform {
  required_version = ">= 1.3.0"
}

variable "same_stack_ssm" {
  type    = any
  default = null
}

variable "same_stack_asm" {
  type    = any
  default = null
}

variable "ssm_query" {
  type    = any
  default = null
}

variable "asm_query" {
  type    = any
  default = null
}

variable "ssm_raw_key" {
  type    = any
  default = null
}

variable "asm_raw_key" {
  type    = any
  default = null
}

variable "template_read" {
  type    = any
  default = null
}

variable "cold_start_default" {
  type    = any
  default = null
}

variable "cross_stack_ssm" {
  type    = any
  default = null
}

variable "cross_stack_asm" {
  type    = any
  default = null
}

variable "cross_stack_ssm_query" {
  type    = any
  default = null
}

variable "cross_stack_template" {
  type    = any
  default = null
}
