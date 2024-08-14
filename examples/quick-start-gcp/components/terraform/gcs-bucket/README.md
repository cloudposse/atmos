# gcs-bucket

Creates a new bucket in Google cloud storage service (GCS) with an option to enable or disable KMS. Once a bucket has
been created, its location can't be changed.

Note: If the project ID is not set on the resource or in the provider block it will be dynamically determined which will
require enabling the compute API.
