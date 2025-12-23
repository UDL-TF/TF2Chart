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

{{- define "tf2chart.workloadName" -}}
{{- $workloadKind := lower (default "deployment" .Values.workload.kind) -}}
{{- if and (eq $workloadKind "deployment") .Values.workload.nameOverride -}}
{{- .Values.workload.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- else if and (eq $workloadKind "statefulset") .Values.statefulSet.nameOverride -}}
{{- .Values.statefulSet.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- include "tf2chart.fullname" . -}}
{{- end -}}
{{- end -}}

{{- define "tf2chart.permissionsInitContainer" -}}
{{- $ctx := . -}}
- name: {{ $ctx.name }}
  image: {{ $ctx.image }}
  imagePullPolicy: {{ $ctx.pullPolicy }}
  securityContext:
    runAsUser: 0
    runAsGroup: 0
    runAsNonRoot: false
  env:
    - name: PERMISSIONS_CONFIG
      value: {{ dict "path" $ctx.path "user" $ctx.user "group" $ctx.group "mode" $ctx.mode | toJson | quote }}
  volumeMounts:
    - name: {{ $ctx.volumeName }}
      mountPath: {{ $ctx.mountPath }}
{{- end -}}
