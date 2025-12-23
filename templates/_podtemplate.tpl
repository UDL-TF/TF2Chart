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
  {{- $permName := default "init-permissions" $permissionsInit.name }}
  {{- $permRunFirst := ne (default true $permissionsInit.runFirst) false }}
  {{- $permRunLast := ne (default true $permissionsInit.runLast) false }}
  {{- $permMountPath := default "/mnt/base" $permissionsInit.mountPath }}
  {{- $permVolumeName := default "host-base" $permissionsInit.volumeName }}
  {{- $permPostPath := default .Values.paths.containerTarget $permissionsInit.postPath }}
  {{- $permPostMount := default .Values.paths.containerTarget $permissionsInit.postMountPath }}
  {{- $permPostVolume := default "view-layer" $permissionsInit.postVolume }}
  {{- $permPostName := default (printf "%s-final" $permName) $permissionsInit.postName }}
  {{- $permUser := default 1000 $permissionsInit.user }}
  {{- $permGroup := default 1000 $permissionsInit.group }}
  {{- $permMode := default "775" $permissionsInit.chmod }}
  {{- $permImage := default "busybox" $permissionsInit.image }}
  {{- $permImagePullPolicy := default "IfNotPresent" $permissionsInit.imagePullPolicy }}
  {{- $permActive := and $permEnabled (or $permRunFirst $permRunLast) }}
  {{- $entrypointCopy := default (dict) .Values.entrypointCopy }}
  {{- $entryImage := default (dict) $entrypointCopy.image }}
  {{- $entryImageRepo := default .Values.app.image.repository $entryImage.repository }}
  {{- $entryImageTag := default .Values.app.image.tag $entryImage.tag }}
  {{- $entryImagePullPolicy := default (default "IfNotPresent" .Values.app.image.pullPolicy) $entryImage.pullPolicy }}
  {{- $entryEnabled := and $mergerEnabled (ne (default true $entrypointCopy.enabled) false) }}
  {{- $entrySource := default "/tf/entrypoint.sh" $entrypointCopy.sourcePath }}
  {{- $entryDest := default (printf "%s/entrypoint.sh" .Values.paths.containerTarget) $entrypointCopy.destinationPath }}
  {{- $entryChmod := default "755" $entrypointCopy.chmod }}
  {{- $entryDestClean := trimPrefix "/" $entryDest }}
  {{- $targetRootBase := trimSuffix "/" .Values.paths.containerTarget }}
  {{- $targetRootStripped := trimPrefix "/" $targetRootBase }}
  {{- $targetPrefix := printf "%s/" $targetRootStripped }}
  {{- $entryDestRel := default "entrypoint.sh" (ternary (trimPrefix $targetPrefix $entryDestClean) $entryDestClean (and (ne $targetRootStripped "") (hasPrefix $targetPrefix $entryDestClean))) }}
  {{- $viewLayerMount := "/view-layer" }}
  {{- $watcherValues := default (dict) .Values.merger.watcher }}
  {{- $watcherImage := default (dict) $watcherValues.image }}
  {{- $watcherEnabled := and $mergerEnabled (ne (default true $watcherValues.enabled) false) }}
  {{- $watcherImageRepo := default .Values.merger.image.repository $watcherImage.repository }}
  {{- $watcherImageTag := default .Values.merger.image.tag $watcherImage.tag }}
  {{- $watcherImagePullPolicy := default .Values.merger.image.pullPolicy $watcherImage.pullPolicy }}
  {{- $watcherName := default "merger-watcher" $watcherValues.name }}
  {{- $writableList := list }}
  {{- range $index, $entry := .Values.writablePaths }}
    {{- $pathVal := "" }}
    {{- $volumeName := "host-base" }}
    {{- $sourceMount := "" }}
    {{- $subPath := "" }}
    {{- if kindIs "string" $entry }}
      {{- $pathVal = $entry }}
      {{- $sourceMount = "/mnt/base" }}
    {{- else if kindIs "map" $entry }}
      {{- $pathVal = default "" $entry.path }}
      {{- $volumeName = default "host-base" $entry.volume }}
      {{- if $entry.overlay }}
        {{- $volumeName = printf "layer-%s" $entry.overlay }}
      {{- end }}
      {{- $subPath = default "" $entry.subPath }}
      {{- $sourceMount = default "" $entry.sourceMount }}
      {{- if and (not $sourceMount) $entry.overlay }}
        {{- $sourceMount = printf "/mnt/overlays/%s" $entry.overlay }}
      {{- end }}
    {{- end }}
    {{- if and (not $sourceMount) (eq $volumeName "host-base") }}
      {{- $sourceMount = "/mnt/base" }}
    {{- else if and (not $sourceMount) (hasPrefix "layer-" $volumeName) }}
      {{- $overlayName := trimPrefix "layer-" $volumeName }}
      {{- $sourceMount = printf "/mnt/overlays/%s" $overlayName }}
    {{- end }}
    {{- $pathClean := trimPrefix "/" $pathVal }}
    {{- if $pathClean }}
      {{- $dict := dict "path" $pathClean "volumeName" $volumeName }}
      {{- if $sourceMount }}
        {{- $_ := set $dict "hostMount" $sourceMount }}
      {{- end }}
      {{- if $subPath }}
        {{- $_ := set $dict "subPath" $subPath }}
      {{- end }}
      {{- $writableList = append $writableList $dict }}
    {{- end }}
  {{- end }}
  {{- $writablePaths := $writableList }}
  {{- $templateCopyList := list }}
  {{- range $index, $entry := .Values.copyTemplates }}
    {{- $targetPath := trimPrefix "/" (default "" $entry.targetPath) }}
    {{- $sourcePath := trimPrefix "/" (default "" $entry.sourcePath) }}
    {{- $sourceMount := default "" $entry.sourceMount }}
    {{- if and (not $sourceMount) $entry.overlay }}
      {{- $sourceMount = printf "/mnt/overlays/%s" $entry.overlay }}
    {{- end }}
    {{- if not $sourceMount }}
      {{- $sourceMount = "/mnt/base" }}
    {{- end }}
    {{- $cleanTarget := ne (default true $entry.cleanTarget) false }}
    {{- if and $targetPath $sourcePath $sourceMount }}
      {{- $dict := dict "targetPath" $targetPath "sourcePath" $sourcePath "sourceMount" $sourceMount "cleanTarget" $cleanTarget }}
      {{- $templateCopyList = append $templateCopyList $dict }}
    {{- end }}
  {{- end }}
  {{- $templateCopies := $templateCopyList }}
  {{- $watchSet := dict }}
  {{- $defaultWatchPaths := list }}
  {{- $watchBase := ne (default false $watcherValues.watchBase) false }}
  {{- if $watchBase }}
    {{- $baseWatch := "/mnt/base" }}
    {{- $defaultWatchPaths = append $defaultWatchPaths $baseWatch }}
    {{- $_ := set $watchSet $baseWatch true }}
  {{- end }}
  {{- range .Values.overlays }}
    {{- $mountPath := printf "/mnt/overlays/%s" .name }}
    {{- if not (hasKey $watchSet $mountPath) }}
      {{- $defaultWatchPaths = append $defaultWatchPaths $mountPath }}
      {{- $_ := set $watchSet $mountPath true }}
    {{- end }}
    {{- $depth := int (default 0 .watchParentDepth) }}
    {{- if gt $depth 0 }}
      {{- $parentPath := trimSuffix "/" $mountPath }}
      {{- range $i, $_depth := until $depth }}
        {{- $parentPath = dir $parentPath }}
        {{- if and $parentPath (ne $parentPath "") }}
          {{- if not (hasKey $watchSet $parentPath) }}
            {{- $defaultWatchPaths = append $defaultWatchPaths $parentPath }}
            {{- $_ := set $watchSet $parentPath true }}
          {{- end }}
        {{- end }}
      {{- end }}
    {{- end }}
    {{- range $extra := (default (list) .extraWatchPaths) }}
    {{- if and $extra (not (hasKey $watchSet $extra)) }}
      {{- $defaultWatchPaths = append $defaultWatchPaths $extra }}
      {{- $_ := set $watchSet $extra true }}
      {{- end }}
    {{- end }}
  {{- end }}
  {{- $configuredWatch := default (list) $watcherValues.watchPaths }}
  {{- $watchPaths := $configuredWatch }}
  {{- if not (gt (len $watchPaths) 0) }}
    {{- $watchPaths = $defaultWatchPaths }}
  {{- end }}
  {{- $watchEvents := default (list "close_write" "create" "delete" "moved_to" "moved_from") $watcherValues.events }}
  {{- $watchEventArg := join "," $watchEvents }}
  {{- $debounceSeconds := default 2 $watcherValues.debounceSeconds }}
  {{- $installInotify := ne (default true $watcherValues.installInotifyTools) false }}
  {{- $pollInterval := default 300 $watcherValues.pollIntervalSeconds }}
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
    {{- $overlayType := default "hostPath" .type }}
    - name: layer-{{ .name }}
      {{- if eq $overlayType "configMap" }}
      configMap:
        name: {{ required (printf "overlay %s requires sourceName" .name) .sourceName }}
      {{- else if eq $overlayType "secret" }}
      secret:
        secretName: {{ required (printf "overlay %s requires sourceName" .name) .sourceName }}
      {{- else if eq $overlayType "pvc" }}
      persistentVolumeClaim:
        claimName: {{ required (printf "overlay %s requires sourceName" .name) .sourceName }}
      {{- else if eq $overlayType "hostPath" }}
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
  {{- if or $mergerEnabled $permActive (gt (len $pre) 0) (gt (len $post) 0) }}
  initContainers:
    {{- if and $permEnabled $permRunFirst }}
    {{- include "tf2chart.permissionsInitContainer" (dict "name" $permName "image" $permImage "pullPolicy" $permImagePullPolicy "path" $permPath "user" $permUser "group" $permGroup "mode" $permMode "volumeName" $permVolumeName "mountPath" $permMountPath) | nindent 4 }}
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
          {{ include "tf2chart.mergeScript" (dict "root" . "writablePaths" $writablePaths "templateCopies" $templateCopies) | indent 10 }}
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
          {{- if .subPath }}
          subPath: {{ .subPath }}
          {{- end }}
        {{- end }}
        {{- with .Values.merger.extraVolumeMounts }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    {{- end }}
    {{- if $entryEnabled }}
    - name: {{ default "init-entrypoint" $entrypointCopy.name }}
      image: {{ printf "%s:%s" $entryImageRepo $entryImageTag }}
      imagePullPolicy: {{ $entryImagePullPolicy }}
      command: ["/bin/sh", "-c"]
      args:
        - |
          set -eu
          SRC="{{ $entrySource }}"
          DEST="{{ $viewLayerMount }}/{{ $entryDestRel }}"
          if [ ! -f "$SRC" ]; then
            echo "Source $SRC not found in init container image" >&2
            exit 1
          fi
          mkdir -p "$(dirname "$DEST")"
          cp "$SRC" "$DEST"
          chmod {{ $entryChmod }} "$DEST"
      {{- with $entrypointCopy.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $entrypointCopy.securityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumeMounts:
        - name: view-layer
          mountPath: {{ $viewLayerMount }}
    {{- end }}
    {{- range $post }}
    {{- toYaml (list .) | nindent 4 }}
    {{- end }}
    {{- if and $permEnabled $permRunLast }}
    {{- include "tf2chart.permissionsInitContainer" (dict "name" $permPostName "image" $permImage "pullPolicy" $permImagePullPolicy "path" $permPostPath "user" $permUser "group" $permGroup "mode" $permMode "volumeName" $permPostVolume "mountPath" $permPostMount) | nindent 4 }}
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
        {{- $overlayReadOnly := default true .readOnly }}
        - name: layer-{{ .name }}
          mountPath: /mnt/overlays/{{ .name }}
          {{- if .subPath }}
          subPath: {{ .subPath }}
          {{- end }}
          {{- if $overlayReadOnly }}
          readOnly: true
          {{- end }}
        {{- end }}
        {{- end }}
        {{- if $mergerEnabled }}
        {{- range $writablePaths }}
        - name: {{ .volumeName }}
          mountPath: {{ $.Values.paths.containerTarget }}/{{ .path }}
          {{- if .subPath }}
          subPath: {{ .subPath }}
          {{- else }}
          subPath: {{ .path }}
          {{- end }}
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
    {{- if $watcherEnabled }}
    - name: {{ $watcherName }}
      image: {{ printf "%s:%s" $watcherImageRepo $watcherImageTag }}
      imagePullPolicy: {{ $watcherImagePullPolicy }}
      {{- if $watcherValues.command }}
      command:
        {{- toYaml $watcherValues.command | nindent 8 }}
      {{- else }}
      command:
        - /bin/sh
        - -c
      {{- end }}
      {{- if $watcherValues.args }}
      args:
        {{- toYaml $watcherValues.args | nindent 8 }}
      {{- else }}
      args:
        - |
          set -euo pipefail
          {{- if $installInotify }}
          if ! command -v inotifywait >/dev/null 2>&1; then
            if command -v apk >/dev/null 2>&1; then
              apk add --no-cache inotify-tools >/dev/null 2>&1 || true
            elif command -v microdnf >/dev/null 2>&1; then
              (microdnf install -y inotify-tools >/dev/null 2>&1) || true
            elif command -v apt-get >/dev/null 2>&1; then
              (apt-get update >/dev/null 2>&1 && apt-get install -y inotify-tools >/dev/null 2>&1) || true
            else
              echo "inotifywait is not installed and automatic installation failed" >&2
            fi
          fi
          {{- end }}
          {{ include "tf2chart.mergeScript" (dict "root" . "writablePaths" $writablePaths "templateCopies" $templateCopies "skipInitialRun" true) | indent 10 }}
          run_merge
          WATCH_PATHS="{{ join " " $watchPaths }}"
          EVENTS="{{ $watchEventArg }}"
          DEBOUNCE={{ $debounceSeconds }}
          POLL={{ $pollInterval }}
          if [ "$POLL" -lt 0 ]; then
            POLL=0
          fi
          ensure_watch_paths() {
            local missing=0
            for path in $WATCH_PATHS; do
              [ -z "$path" ] && continue
              if [ -d "$path" ]; then
                continue
              fi
              if [ ! -e "$path" ]; then
                mkdir -p "$path" 2>/dev/null || true
                if [ ! -e "$path" ]; then
                  echo "Watch path $path is not available yet; waiting for producer" >&2
                  missing=1
                  continue
                fi
              fi
            done
            return $missing
          }
          if ! ensure_watch_paths; then
            echo "Waiting for initial watch paths to exist..." >&2
          fi
          if command -v inotifywait >/dev/null 2>&1; then
            HAS_INOTIFY=1
          else
            HAS_INOTIFY=0
            echo "inotifywait not available; falling back to polling mode" >&2
          fi
          while true; do
            if ! ensure_watch_paths; then
              sleep "$DEBOUNCE"
              continue
            fi
            if [ "$HAS_INOTIFY" -eq 1 ]; then
              if [ "$POLL" -gt 0 ]; then
                if inotifywait -qq -r -t "$POLL" -e "$EVENTS" $WATCH_PATHS >/dev/null 2>&1; then
                  sleep "$DEBOUNCE"
                  run_merge
                  continue
                fi
                STATUS=$?
                if [ "$STATUS" -eq 2 ]; then
                  echo "inotify timeout after $POLL seconds; running periodic merge"
                  run_merge
                  continue
                fi
                echo "inotifywait failed with status $STATUS; switching to polling mode" >&2
                HAS_INOTIFY=0
              else
                if inotifywait -qq -r -e "$EVENTS" $WATCH_PATHS >/dev/null 2>&1; then
                  sleep "$DEBOUNCE"
                  run_merge
                  continue
                fi
                STATUS=$?
                echo "inotifywait failed with status $STATUS; switching to polling mode" >&2
                HAS_INOTIFY=0
              fi
            fi
            if [ "$POLL" -gt 0 ]; then
              sleep "$POLL"
            else
              sleep "$DEBOUNCE"
            fi
            run_merge
          done
      {{- end }}
      {{- with $watcherValues.env }}
      env:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumeMounts:
        - name: host-base
          mountPath: /mnt/base
          readOnly: true
        - name: view-layer
          mountPath: {{ .Values.paths.containerTarget }}
        {{- range .Values.overlays }}
        {{- $overlayReadOnly := default true .readOnly }}
        - name: layer-{{ .name }}
          mountPath: /mnt/overlays/{{ .name }}
          {{- if .subPath }}
          subPath: {{ .subPath }}
          {{- end }}
          {{- if $overlayReadOnly }}
          readOnly: true
          {{- end }}
        {{- end }}
        {{- with $watcherValues.extraVolumeMounts }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- with $watcherValues.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $watcherValues.securityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
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
