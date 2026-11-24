apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-capabilities
data:
  kubeVersion: {{ .Capabilities.KubeVersion.Version | quote }}
