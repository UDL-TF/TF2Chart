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
  command:
    - sh
    - -c
  args:
    - |
      set -e
      TARGET="{{ $ctx.path }}"
      chown -R {{ $ctx.user }}:{{ $ctx.group }} "$TARGET"
      chmod -R {{ $ctx.mode }} "$TARGET"
  volumeMounts:
    - name: {{ $ctx.volumeName }}
      mountPath: {{ $ctx.mountPath }}
{{- end -}}

{{- define "tf2chart.mergeScript" -}}
{{- $root := .root -}}
{{- $values := $root.Values -}}
{{- $writable := default (list) .writablePaths -}}
{{- $copyTemplates := default (list) .templateCopies -}}
{{- $skipInitial := default false .skipInitialRun -}}
{{- $perm := default (dict) $values.permissionsInit -}}
{{- $permUser := default 1000 $perm.user -}}
{{- $permGroup := default 1000 $perm.group -}}
{{- $permMode := default "775" $perm.chmod -}}
{{- $fixViewLayer := ne (default false $perm.applyDuringMerge) false -}}
{{- $viewPath := default $values.paths.containerTarget $perm.postPath -}}
{{- $applyPaths := default (list) $perm.applyPaths -}}
{{- if and $fixViewLayer (eq (len $applyPaths) 0) -}}
  {{- $applyPaths = append $applyPaths $viewPath -}}
{{- end -}}
merge_dir() {
  local src="$1"
  local dest="$2"
  if [ ! -d "$src" ]; then
    echo "Warning: Source $src does not exist; skipping."
    return
  fi
  mkdir -p "$dest"
  (cd "$src" && find . -type d -print0) | while IFS= read -r -d '' dir; do
    rel="${dir#./}"
    if [ -z "$rel" ]; then
      continue
    fi
    mkdir -p "$dest/$rel"
  done
  (cd "$src" && find . -type f -print0) | while IFS= read -r -d '' file; do
    rel="${file#./}"
    [ -n "$rel" ] || continue
    mkdir -p "$(dirname "$dest/$rel")"
    ln -sf "$src/$rel" "$dest/$rel"
  done
}

copy_template_dir() {
  local src="$1"
  local dest="$2"
  local clean="${3:-true}"
  if [ ! -d "$src" ]; then
    echo "Warning: Template source $src does not exist; skipping copy."
    return
  fi
  if [ "$clean" = "true" ]; then
    rm -rf "$dest"
  fi
  mkdir -p "$dest"
  if [ -z "$(ls -A "$src" 2>/dev/null)" ]; then
    return
  fi
  cp -a "$src/." "$dest/"
}

run_merge() {
  TARGET="{{ $values.paths.containerTarget }}/tf"
  TARGET_BASE="{{ $values.paths.containerTarget }}"
  BASE="/mnt/base"
  echo "--- tf2chart merge starting ---"
  echo "Merging base from $BASE into $TARGET_BASE"
  merge_dir "$BASE" "$TARGET_BASE"
  {{- if $values.overlays }}
  echo "Merging overlays into $TARGET"
  {{- range $values.overlays }}
  {{- $sourcePath := trimPrefix "/" (default "" .sourcePath) }}
  {{- $source := ternary (printf "/mnt/overlays/%s/%s" .name $sourcePath) (printf "/mnt/overlays/%s" .name) (ne $sourcePath "") }}
  echo "Layer: {{ .name }} -> {{ $source }}"
  merge_dir "{{ $source }}" "$TARGET"
  {{- end }}
  {{- end }}
  {{- if $writable }}
  echo "Ensuring writable passthroughs"
  {{- range $writable }}
  mkdir -p "$TARGET/{{ .path }}"
  {{- if .hostMount }}
  if ! mkdir -p "{{ .hostMount }}/{{ .path }}" 2>/dev/null; then
    echo "Warning: unable to prepare source {{ .hostMount }}/{{ .path }} (insufficient permissions?)"
  fi
  {{- else }}
  echo "Warning: host mount for {{ .path }} is not defined; skipping source mkdir"
  {{- end }}
  {{- end }}
  {{- end }}
  {{- if $copyTemplates }}
  echo "Copying template directories"
  {{- range $copyTemplates }}
  copy_template_dir "{{ .sourceMount }}/{{ .sourcePath }}" "$TARGET_BASE/{{ .targetPath }}" {{ if .cleanTarget }}"true"{{ else }}"false"{{ end }}
  {{- end }}
  {{- end }}
  echo "--- 4. Removing dangling symlinks ---"
  find "$TARGET_BASE" -xtype l -delete 2>/dev/null || true
  find "$TARGET" -xtype l -delete 2>/dev/null || true
  {{- if $fixViewLayer }}
  echo "--- 5. Re-applying permissions ---"
  {{- range $applyPaths }}
  if [ -e "{{ . }}" ]; then
    chown -R {{ $permUser }}:{{ $permGroup }} "{{ . }}" 2>/dev/null || echo "Warning: unable to chown {{ . }}"
    chmod -R {{ $permMode }} "{{ . }}" 2>/dev/null || echo "Warning: unable to chmod {{ . }}"
  else
    echo "Warning: permission target {{ . }} is missing"
  fi
  {{- end }}
  {{- end }}
  echo "--- tf2chart merge complete ---"
}

{{- if not $skipInitial }}
run_merge
{{- end }}
{{- end -}}
