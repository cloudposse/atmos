{{- if .Capabilities.APIVersions.Has "monitoring.coreos.com/v1/ServiceMonitor" }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ .Release.Name }}
spec:
  selector:
    matchLabels:
      app: {{ .Release.Name }}
  endpoints:
    - port: http
{{- end }}
