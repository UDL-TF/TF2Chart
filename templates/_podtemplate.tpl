{{- define "tf2chart.podTemplate" -}}
metadata:
  labels:
    {{- include "tf2chart.selectorLabels" . | nindent 4 }}
    {{- with .Values.podLabels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
  {{- with .Values.podAnnotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  {{- if .Values.hostNetwork }}
  hostNetwork: true
  dnsPolicy: {{ default "ClusterFirstWithHostNet" .Values.dnsPolicy }}
  {{- else if .Values.dnsPolicy }}
  dnsPolicy: {{ .Values.dnsPolicy }}
  {{- end }}
  {{- with .Values.serviceAccountName }}
  serviceAccountName: {{ . }}
  {{- end }}
  {{- $mergerEnabled := ne (default true .Values.merger.enabled) false }}
  {{- $contentVolume := ternary "view-layer" "host-base" $mergerEnabled }}
  {{- $permissionsInit := default (dict) .Values.permissionsInit }}
  {{- $permEnabled := and (ne (default true $permissionsInit.enabled) false) true }}
  {{- $permPath := default "/mnt/base" $permissionsInit.path }}
  {{- $permUser := default 1000 $permissionsInit.user }}
  {{- $permGroup := default 1000 $permissionsInit.group }}
  {{- $permMode := default "775" $permissionsInit.chmod }}
  {{- with .Values.podSecurityContext }}
  securityContext:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  volumes:
    - name: host-base
      hostPath:
        path: {{ required "paths.hostSource is required" .Values.paths.hostSource }}
        type: {{ .Values.paths.hostPathType | default "Directory" }}
    - name: view-layer
      emptyDir: {}
    {{- range .Values.overlays }}
    - name: layer-{{ .name }}
      {{- if eq .type "configMap" }}
      configMap:
        name: {{ required (printf "overlay %s requires sourceName" .name) .sourceName }}
      {{- else if eq .type "secret" }}
      secret:
        secretName: {{ required (printf "overlay %s requires sourceName" .name) .sourceName }}
      {{- else if eq .type "pvc" }}
      persistentVolumeClaim:
        claimName: {{ required (printf "overlay %s requires sourceName" .name) .sourceName }}
      {{- else if eq .type "hostPath" }}
      hostPath:
        path: {{ required (printf "overlay %s requires path" .name) .path }}
        type: {{ .hostPathType | default "Directory" }}
      {{- end }}
    {{- end }}
    {{- with .Values.extraVolumes }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
  {{- $pre := default (list) .Values.initContainers.pre }}
  {{- $post := default (list) .Values.initContainers.post }}
  {{- if or $mergerEnabled $permEnabled (gt (len $pre) 0) (gt (len $post) 0) }}
  initContainers:
    {{- if $permEnabled }}
    - name: {{ default "init-permissions" $permissionsInit.name }}
      image: {{ default "busybox" $permissionsInit.image }}
      imagePullPolicy: {{ default "IfNotPresent" $permissionsInit.imagePullPolicy }}
      command:
        - sh
        - -c
      args:
        - |
          set -e
          TARGET="{{ $permPath }}"
          chown -R {{ $permUser }}:{{ $permGroup }} "$TARGET"
          chmod -R {{ $permMode }} "$TARGET"
      volumeMounts:
        - name: host-base
          mountPath: /mnt/base
    {{- end }}
    {{- range $pre }}
    {{- toYaml (list .) | nindent 4 }}
    {{- end }}
    {{- if $mergerEnabled }}
    - name: stitcher
      image: {{ printf "%s:%s" .Values.merger.image.repository .Values.merger.image.tag }}
      imagePullPolicy: {{ .Values.merger.image.pullPolicy }}
      command: ["/bin/sh", "-c"]
      args:
        - |
          set -eu
          TARGET="{{ .Values.paths.containerTarget }}/tf"
          TARGET_BASE="{{ .Values.paths.containerTarget }}"
          BASE="/mnt/base"
          echo "Preparing dynamic view at $TARGET"
          merge_dir() {
            local src="$1"
            local dest="$2"
            if [ ! -d "$src" ]; then
              echo "Warning: Source $src does not exist or is empty."
              return
            fi
            cd "$src"
            find . -type d -print | while read -r dir; do
              rel="${dir#./}"
              mkdir -p "$dest/$rel"
            done
            find . -type f -print | while read -r file; do
              rel="${file#./}"
              mkdir -p "$(dirname "$dest/$rel")"
              ln -sf "$src/$rel" "$dest/$rel"
            done
          }

          echo "--- 1. Merging Base Layer ---"
          merge_dir "$BASE" "$TARGET_BASE"

          echo "--- 2. Merging Overlays ---"
          {{- range .Values.overlays }}
          echo "Processing layer: {{ .name }} ({{ .type }})"
          merge_dir "/mnt/overlays/{{ .name }}" "$TARGET"
          {{- end }}

          echo "--- 3. Preparing Writable Passthroughs ---"
          {{- range .Values.writablePaths }}
          mkdir -p "$TARGET/{{ . }}"
          mkdir -p "$BASE/{{ . }}"
          {{- end }}

          echo "--- Setup Complete ---"
      {{- with .Values.merger.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.merger.securityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.merger.extraEnv }}
      env:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumeMounts:
        - name: host-base
          mountPath: /mnt/base
        - name: view-layer
          mountPath: {{ .Values.paths.containerTarget }}
        {{- range .Values.overlays }}
        - name: layer-{{ .name }}
          mountPath: /mnt/overlays/{{ .name }}
        {{- end }}
        {{- with .Values.merger.extraVolumeMounts }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    {{- end }}
    {{- range $post }}
    {{- toYaml (list .) | nindent 4 }}
    {{- end }}
  {{- end }}
  containers:
    - name: app
      image: {{ printf "%s:%s" .Values.app.image.repository .Values.app.image.tag }}
      imagePullPolicy: {{ .Values.app.image.pullPolicy }}
      {{- with .Values.app.command }}
      command:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.app.args }}
      args:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      ports:
        {{- if .Values.app.ports }}
        {{- range $index, $port := .Values.app.ports }}
        - name: {{ $port.name | default (printf "port-%v" $index) }}
          containerPort: {{ required (printf "app.ports[%d].containerPort is required" $index) $port.containerPort }}
          protocol: {{ $port.protocol | default "TCP" }}
          {{- if $port.hostPort }}
          hostPort: {{ $port.hostPort }}
          {{- end }}
        {{- end }}
        {{- else }}
        - name: http
          containerPort: {{ .Values.app.containerPort }}
          protocol: TCP
        {{- end }}
      {{- with .Values.app.env }}
      env:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumeMounts:
        - name: {{ $contentVolume }}
          mountPath: {{ .Values.paths.containerTarget }}
        {{- if $mergerEnabled }}
        - name: host-base
          mountPath: /mnt/base
          readOnly: true
        {{- range .Values.overlays }}
        - name: layer-{{ .name }}
          mountPath: /mnt/overlays/{{ .name }}
          readOnly: true
        {{- end }}
        {{- end }}
        {{- if $mergerEnabled }}
        {{- range .Values.writablePaths }}
        - name: host-base
          mountPath: {{ $.Values.paths.containerTarget }}/{{ . }}
          subPath: {{ . }}
        {{- end }}
        {{- end }}
        {{- with .Values.app.extraVolumeMounts }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- if .Values.app.stdin }}
      stdin: {{ .Values.app.stdin }}
      {{- end }}
      {{- if .Values.app.tty }}
      tty: {{ .Values.app.tty }}
      {{- end }}
      {{- with .Values.app.livenessProbe }}
      livenessProbe:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.app.readinessProbe }}
      readinessProbe:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.app.startupProbe }}
      startupProbe:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.app.lifecycle }}
      lifecycle:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.app.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.app.securityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
  {{- with .Values.priorityClassName }}
  priorityClassName: {{ . }}
  {{- end }}
  {{- with .Values.nodeSelector }}
  nodeSelector:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .Values.tolerations }}
  tolerations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .Values.affinity }}
  affinity:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
