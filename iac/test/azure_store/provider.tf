terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "4.65.0"
    }
  }

  backend "local" {}
}

provider "azurerm" {
  # Configuration options
}