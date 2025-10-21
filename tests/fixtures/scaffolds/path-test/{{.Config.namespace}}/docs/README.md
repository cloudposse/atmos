# {{.Config.namespace}} Documentation

This is the documentation for the **{{.Config.namespace}}** namespace.

## Author
Created by: {{.Config.author | default "Unknown Author"}}

## Project Details
- Template Name: {{.TemplateName}}
- Scaffold Path: {{.ScaffoldPath}}
- Namespace: {{.Config.namespace}}

## Description
{{.Config.description | default "No description provided"}}
