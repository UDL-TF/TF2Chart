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
  {{- $decompressor := default (dict) .Values.decompressor }}
  {{- $decompEnabled := and (hasKey .Values "decompressor") (ne (default false $decompressor.enabled) false) }}
  {{- $decompImage := default (dict) $decompressor.image }}
  {{- $decompImageRepo := default "ghcr.io/udl-tf/tf2chart-decompressor" $decompImage.repository }}
  {{- $decompImageTag := default "latest" $decompImage.tag }}
  {{- $decompImagePullPolicy := default "Always" $decompImage.pullPolicy }}
  {{- $decompScanBase := true }}
  {{- if hasKey $decompressor "scanBase" }}
    {{- $decompScanBase = $decompressor.scanBase }}
  {{- end }}
  {{- $decompScanOverlays := default (list) $decompressor.scanOverlays }}
  {{- $decompCache := default (dict) $decompressor.cache }}
  {{- $decompCacheEnabled := ne (default false $decompCache.enabled) false }}
  {{- $decompCacheType := default "pvc" $decompCache.type }}
  {{- $decompCacheHostPath := default "/var/lib/tf2/decomp-cache" $decompCache.hostPath }}
  {{- $decompCacheHostPathType := default "DirectoryOrCreate" $decompCache.hostPathType }}
  {{- $permissionsInit := default (dict) .Values.permissionsInit }}
  {{- $permEnabled := and (ne (default true $permissionsInit.enabled) false) true }}
  {{- $permPathRaw := default "" $permissionsInit.path }}
  {{- $permPath := ternary $permPathRaw "/mnt/base" (ne (trimAll " " $permPathRaw) "") }}
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
  {{- $permImagePullPolicy := default "Always" $permissionsInit.imagePullPolicy }}
  {{- $fixViewLayer := ne (default false $permissionsInit.applyDuringMerge) false }}
  {{- $viewPath := default .Values.paths.containerTarget $permissionsInit.postPath }}
  {{- $applyPaths := default (list) $permissionsInit.applyPaths }}
  {{- if and $fixViewLayer (eq (len $applyPaths) 0) }}
    {{- $applyPaths = append $applyPaths $viewPath }}
  {{- end }}
  {{- $permActive := and $permEnabled (or $permRunFirst $permRunLast) }}
  {{- $entrypointCopy := default (dict) .Values.entrypointCopy }}
  {{- $entryImage := default (dict) $entrypointCopy.image }}
  {{- $entryImageRepo := default .Values.app.image.repository $entryImage.repository }}
  {{- $entryImageTag := default .Values.app.image.tag $entryImage.tag }}
  {{- $entryImagePullPolicy := default (default "Always" .Values.app.image.pullPolicy) $entryImage.pullPolicy }}
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
  {{- $watcherImagePullPolicy := default (default "Always" .Values.merger.image.pullPolicy) $watcherImage.pullPolicy }}
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
      {{- if and (kindIs "map" $entry) $entry.template }}
        {{- $templateSourceMount := default "" $entry.template.sourceMount }}
        {{- if and (not $templateSourceMount) $entry.template.overlay }}
          {{- $templateSourceMount = printf "/mnt/overlays/%s" $entry.template.overlay }}
        {{- end }}
        {{- if not $templateSourceMount }}
          {{- $templateSourceMount = "/mnt/base" }}
        {{- end }}
        {{- $templateSourcePath := trimPrefix "/" (default $pathClean $entry.template.sourcePath) }}
        {{- $templateClean := ne (default true $entry.template.clean) false }}
        {{- $templateDict := dict "sourceMount" $templateSourceMount "sourcePath" $templateSourcePath "clean" $templateClean }}
        {{- $_ := set $dict "template" $templateDict }}
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
    {{- $targetMode := lower (default "view" $entry.targetMode) }}
    {{- $onlyOnInit := ne (default false $entry.onlyOnInit) false }}
    {{- if and $targetPath $sourcePath $sourceMount }}
      {{- $dict := dict "targetPath" $targetPath "sourcePath" $sourcePath "sourceMount" $sourceMount "clean" $cleanTarget "targetMode" $targetMode "onlyOnInit" $onlyOnInit }}
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
  {{- $globalWatchParentDepth := int (default 0 $watcherValues.watchParentDepth) }}
  {{- range .Values.overlays }}
    {{- $mountPath := printf "/mnt/overlays/%s" .name }}
    {{- if not (hasKey $watchSet $mountPath) }}
      {{- $defaultWatchPaths = append $defaultWatchPaths $mountPath }}
      {{- $_ := set $watchSet $mountPath true }}
    {{- end }}
    {{- if gt $globalWatchParentDepth 0 }}
      {{- $parentPath := trimSuffix "/" $mountPath }}
      {{- range $i, $_depth := until $globalWatchParentDepth }}
        {{- $parentPath = dir $parentPath }}
        {{- if and $parentPath (ne $parentPath "") }}
          {{- if not (hasKey $watchSet $parentPath) }}
            {{- $defaultWatchPaths = append $defaultWatchPaths $parentPath }}
            {{- $_ := set $watchSet $parentPath true }}
          {{- end }}
        {{- end }}
      {{- end }}
    {{- end }}
  {{- end }}
  {{- range $extra := (default (list) $watcherValues.extraWatchPaths) }}
    {{- if and $extra (not (hasKey $watchSet $extra)) }}
      {{- $defaultWatchPaths = append $defaultWatchPaths $extra }}
      {{- $_ := set $watchSet $extra true }}
    {{- end }}
  {{- end }}
  {{- $configuredWatch := default (list) $watcherValues.watchPaths }}
  {{- $watchPaths := $configuredWatch }}
  {{- if not (gt (len $watchPaths) 0) }}
    {{- $watchPaths = $defaultWatchPaths }}
  {{- end }}
  {{- $watchEvents := default (list "close_write" "create" "delete" "moved_to" "moved_from") $watcherValues.events }}
  {{- $debounceSeconds := default 2 $watcherValues.debounceSeconds }}
  {{- $pollInterval := default 300 $watcherValues.pollIntervalSeconds }}
  {{- $targetBasePath := trimSuffix "/" .Values.paths.containerTarget }}
  {{- if eq $targetBasePath "" }}
    {{- $targetBasePath = "/" }}
  {{- end }}
  {{- $targetContentPath := ternary (printf "%s/tf" $targetBasePath) "/tf" (ne $targetBasePath "/") }}
  {{- $overlayConfigs := list }}
  {{- /* Add user-defined overlays first */ -}}
  {{- range .Values.overlays }}
    {{- $sourcePath := trimPrefix "/" (default "" .sourcePath) }}
    {{- $baseMount := printf "/mnt/overlays/%s" .name }}
    {{- $overlaySource := ternary (printf "%s/%s" $baseMount $sourcePath) $baseMount (ne $sourcePath "") }}
    {{- $overlayConfigs = append $overlayConfigs (dict "name" .name "sourcePath" $overlaySource) }}
  {{- end }}
  {{- /* Add cache as last overlay if enabled and mountAsOverlay is true */ -}}
  {{- if and $decompCacheEnabled (ne (default true $decompCache.mountAsOverlay) false) }}
    {{- $cacheOverlayName := default "decomp-cache" $decompCache.overlayName }}
    {{- $cacheMount := printf "/mnt/overlays/%s" $cacheOverlayName }}
    {{- $overlayConfigs = append $overlayConfigs (dict "name" $cacheOverlayName "sourcePath" $cacheMount) }}
  {{- end }}
  {{- $excludePaths := list }}
  {{- range .Values.copyTemplates }}
    {{- if .onlyOnInit }}
      {{- $targetPath := trimPrefix "/" (default "" .targetPath) }}
      {{- $targetMode := lower (default "view" .targetMode) }}
      {{- if eq $targetMode "writable" }}
        {{- $excludeRel := $targetPath }}
        {{- $prefix := printf "%s/" (trimSuffix "/" $targetRootStripped) }}
        {{- if and (ne $targetRootStripped "") (hasPrefix $prefix $excludeRel) }}
          {{- $excludeRel = trimPrefix $prefix $excludeRel }}
        {{- end }}
        {{- $excludePaths = append $excludePaths $excludeRel }}
      {{- end }}
    {{- end }}
  {{- end }}
  {{- $mergePermissions := dict "applyDuringMerge" $fixViewLayer "applyPaths" $applyPaths "user" $permUser "group" $permGroup "mode" $permMode }}
  {{- $decompressPaths := default (list) .Values.merger.decompressPaths }}
  {{- /* Build a set of overlay names that need write access for decompression */ -}}
  {{- $decompWritableOverlays := dict }}
  {{- range $decompPath := $decompressPaths }}
    {{- /* Check if this decompress path matches any overlay mount path */ -}}
    {{- range $overlay := $.Values.overlays }}
      {{- $overlayMount := printf "/mnt/overlays/%s" $overlay.name }}
      {{- if hasPrefix $overlayMount $decompPath }}
        {{- $_ := set $decompWritableOverlays $overlay.name true }}
      {{- else if hasPrefix $decompPath $overlayMount }}
        {{- $_ := set $decompWritableOverlays $overlay.name true }}
      {{- end }}
    {{- end }}
  {{- end }}
  {{- $mergeConfig := dict "basePath" "/mnt/base" "targetBase" $targetBasePath "targetContent" $targetContentPath "overlays" $overlayConfigs "writablePaths" $writablePaths "copyTemplates" $templateCopies "permissions" $mergePermissions "excludePaths" $excludePaths "decompressPaths" $decompressPaths }}
  {{- $watcherConfig := dict "watchPaths" $watchPaths "events" $watchEvents "debounceSeconds" $debounceSeconds "pollIntervalSeconds" $pollInterval }}
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
    {{- if and $decompEnabled $decompCacheEnabled }}
    - name: decomp-cache
      {{- if eq $decompCacheType "pvc" }}
      persistentVolumeClaim:
        claimName: {{ include "tf2chart.fullname" . }}-decomp-cache
      {{- else if eq $decompCacheType "hostPath" }}
      hostPath:
        path: {{ $decompCacheHostPath }}
        type: {{ $decompCacheHostPathType }}
      {{- end }}
    {{- end }}
    {{- with .Values.extraVolumes }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
  {{- $pre := default (list) .Values.initContainers.pre }}
  {{- $post := default (list) .Values.initContainers.post }}
  {{- if or $mergerEnabled $permActive $decompEnabled (gt (len $pre) 0) (gt (len $post) 0) }}
  initContainers:
    {{- if and $permEnabled $permRunFirst }}
    {{- include "tf2chart.permissionsInitContainer" (dict "name" $permName "image" $permImage "pullPolicy" $permImagePullPolicy "path" $permPath "user" $permUser "group" $permGroup "mode" $permMode "volumeName" $permVolumeName "mountPath" $permMountPath) | nindent 4 }}
    {{- end }}
    {{- range $pre }}
    {{- toYaml (list .) | nindent 4 }}
    {{- end }}
    {{- if $decompEnabled }}
    - name: decompressor
      image: {{ printf "%s:%s" $decompImageRepo $decompImageTag }}
      imagePullPolicy: {{ $decompImagePullPolicy }}
      args:
        {{- if $decompScanBase }}
        - -base=/mnt/base
        {{- end }}
        {{- if gt (len $decompScanOverlays) 0 }}
        {{- $overlayPaths := list }}
        {{- range $overlayName := $decompScanOverlays }}
        {{- $overlayPath := printf "/mnt/overlays/%s" $overlayName }}
        {{- /* Find the overlay config to get sourcePath */ -}}
        {{- $sourcePath := "" }}
        {{- range $.Values.overlays }}
        {{- if eq .name $overlayName }}
        {{- $sourcePath = default "" .sourcePath }}
        {{- end }}
        {{- end }}
        {{- /* Build overlay path with optional subpath */ -}}
        {{- if $sourcePath }}
        {{- $overlayPaths = append $overlayPaths (printf "%s:%s" $overlayPath $sourcePath) }}
        {{- else }}
        {{- $overlayPaths = append $overlayPaths $overlayPath }}
        {{- end }}
        {{- end }}
        - {{ printf "-overlays=%s" (join "," $overlayPaths) }}
        {{- end }}
        {{- if $decompCacheEnabled }}
        - -output=/mnt/decomp-cache
        {{- end }}
      {{- with $decompressor.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $decompressor.securityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumeMounts:
        {{- if $decompScanBase }}
        - name: host-base
          mountPath: /mnt/base
        {{- end }}
        {{- range $decompScanOverlays }}
        - name: layer-{{ . }}
          mountPath: /mnt/overlays/{{ . }}
        {{- end }}
        {{- if $decompCacheEnabled }}
        - name: decomp-cache
          mountPath: /mnt/decomp-cache
        {{- end }}
        {{- with $decompressor.extraVolumeMounts }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    {{- end }}
    {{- if $mergerEnabled }}
    - name: stitcher
      image: {{ printf "%s:%s" .Values.merger.image.repository .Values.merger.image.tag }}
      imagePullPolicy: {{ .Values.merger.image.pullPolicy }}
      env:
        - name: MERGER_CONFIG
          value: {{ $mergeConfig | toJson | quote }}
        {{- with .Values.merger.extraEnv }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- with .Values.merger.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.merger.securityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumeMounts:
        - name: host-base
          mountPath: /mnt/base
        - name: view-layer
          mountPath: {{ .Values.paths.containerTarget }}
        {{- if and $decompCacheEnabled (ne (default true $decompCache.mountAsOverlay) false) }}
        {{- $cacheOverlayName := default "decomp-cache" $decompCache.overlayName }}
        - name: decomp-cache
          mountPath: /mnt/overlays/{{ $cacheOverlayName }}
        {{- end }}
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
        {{- if and $decompCacheEnabled (ne (default true $decompCache.mountAsOverlay) false) }}
        {{- $cacheOverlayName := default "decomp-cache" $decompCache.overlayName }}
        - name: decomp-cache
          mountPath: /mnt/overlays/{{ $cacheOverlayName }}
          readOnly: true
        {{- end }}
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
      {{- with $watcherValues.command }}
      command:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $watcherValues.args }}
      args:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      env:
        - name: MERGER_CONFIG
          value: {{ $mergeConfig | toJson | quote }}
        - name: WATCHER_CONFIG
          value: {{ $watcherConfig | toJson | quote }}
        {{- with $watcherValues.env }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      volumeMounts:
        - name: host-base
          mountPath: /mnt/base
          readOnly: true
        - name: view-layer
          mountPath: {{ .Values.paths.containerTarget }}
        {{- if and $decompCacheEnabled (ne (default true $decompCache.mountAsOverlay) false) }}
        {{- $cacheOverlayName := default "decomp-cache" $decompCache.overlayName }}
        - name: decomp-cache
          mountPath: /mnt/overlays/{{ $cacheOverlayName }}
          readOnly: true
        {{- end }}
        {{- range .Values.overlays }}
        {{- $overlayReadOnly := default true .readOnly }}
        {{- if hasKey $decompWritableOverlays .name }}
          {{- $overlayReadOnly = false }}
        {{- end }}
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
