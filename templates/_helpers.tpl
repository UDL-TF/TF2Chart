{{- define "tf2chart.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "tf2chart.fullname" -}}
{{- printf "%s" (include "tf2chart.name" .) -}}
{{- end -}}

{{- define "tf2chart.labels" -}}
app.kubernetes.io/name: {{ include "tf2chart.name" . }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "tf2chart.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tf2chart.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "tf2chart.serviceName" -}}
{{- if .Values.service.nameOverride -}}
{{- .Values.service.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- else if .Values.statefulSet.serviceName -}}
{{- .Values.statefulSet.serviceName -}}
{{- else -}}
{{- include "tf2chart.fullname" . -}}
{{- end -}}
{{- end -}}
