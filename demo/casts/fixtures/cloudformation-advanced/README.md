# CloudFormation publish cast fixture

This isolated fixture records the advanced CloudFormation publish demo. It
keeps the build, archive, S3 publish, deploy, and HTTP validation hooks from
the product example, with a short publish summary for the cast. The product
example's unit test and end-to-end test harness stay outside the recording so
the visible commands follow the deployment story.
