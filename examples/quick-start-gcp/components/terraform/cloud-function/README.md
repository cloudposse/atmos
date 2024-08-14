# cloud-function

The Terraform module handles the deployment of Cloud Functions (Gen 2) on GCP.

The resources/services/activations/deletions that this module will create/trigger are:

* Deploy Cloud Functions (2nd Gen) with provided source code and trigger

* Provide Cloud Functions Invoker or Developer roles to the users and service accounts

It also creates a new bucket in Google cloud storage service (GCS) with an option to enable or disable kms, when user
didn't pass the name of existing bucket. Once a bucket has been created, its location can't be changed. It acts as a
source to the Google Cloud-Function

Note: If the project id is not set on the resource or in the provider block it will be dynamically determined which will
require enabling the compute api.
