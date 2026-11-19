packer {
  required_plugins {
    hyperv = {
      source  = "github.com/hashicorp/hyperv"
      version = ">= 1.1.4"
    }
    windows-optimize = {
      source  = "github.com/larskroghdk/windows-optimize"
      version = ">= 1.0.0"
    }
  }
}

variable "base_vm" {
  type        = string
  description = "Existing Hyper-V VM with Windows installed and WinRM enabled. It is cloned, never changed."
}

variable "template_name" {
  type        = string
  description = "Name of the VM that is built and exported."
}

variable "switch_name" {
  type    = string
  default = "Default Switch"
}

variable "output_directory" {
  type    = string
  default = "output"
}

variable "winrm_username" {
  type    = string
  default = "Administrator"
}

variable "winrm_password" {
  type      = string
  sensitive = true
}

variable "profile" {
  type    = string
  default = "recommended"
}

variable "exclude" {
  type    = list(string)
  default = []
}

source "hyperv-vmcx" "windows" {
  clone_from_vm_name = var.base_vm
  vm_name            = var.template_name
  switch_name        = var.switch_name
  generation         = 2
  cpus               = 2
  memory             = 4096
  output_directory   = "${var.output_directory}/${var.template_name}"
  headless           = true

  communicator   = "winrm"
  winrm_username = var.winrm_username
  winrm_password = var.winrm_password
  winrm_use_ntlm = true
  winrm_insecure = true
  winrm_timeout  = "30m"

  shutdown_command = "C:\\Windows\\System32\\Sysprep\\sysprep.exe /generalize /oobe /shutdown /quiet /mode:vm"
  shutdown_timeout = "30m"
}

build {
  sources = ["source.hyperv-vmcx.windows"]

  provisioner "windows-optimize" {
    profile     = var.profile
    exclude     = var.exclude
    dry_run     = true
    report_path = "reports/${var.template_name}-plan.json"
  }

  provisioner "windows-optimize" {
    profile            = var.profile
    exclude            = var.exclude
    reboot_if_required = true
    report_path        = "reports/${var.template_name}.json"
  }
}
