variable "host" {
  type = string
}

variable "username" {
  type    = string
  default = "Administrator"
}

variable "password" {
  type      = string
  sensitive = true
}

variable "use_ssl" {
  type    = bool
  default = false
}

variable "profile" {
  type    = string
  default = "recommended"
}

variable "dry_run" {
  type    = bool
  default = true
}

variable "rollback" {
  type    = bool
  default = false
}

variable "report_path" {
  type = string
}

source "null" "windows" {
  communicator   = "winrm"
  winrm_host     = var.host
  winrm_username = var.username
  winrm_password = var.password
  winrm_use_ssl  = var.use_ssl
  winrm_insecure = true
  winrm_use_ntlm = true
  winrm_timeout  = "5m"
}

build {
  sources = ["source.null.windows"]

  provisioner "windows-optimize" {
    profile     = var.rollback ? null : var.profile
    rollback    = var.rollback
    dry_run     = var.dry_run
    report_path = var.report_path
  }
}
