package comrad

import "strings"

func (w *Worker) llamaModelArtifacts(profile WorkloadProfile) (string, []modelSupportArtifact) {
	w.mu.Lock()
	assignment := w.assigns[assignmentKey(profile)]
	cache := map[string]string{}
	for k, v := range w.cache {
		cache[k] = v
	}
	w.mu.Unlock()
	modelPath := ""
	support := []modelSupportArtifact{}
	for _, artifact := range assignment.Artifacts {
		path := cache[artifact.ID]
		if path == "" {
			continue
		}
		if modelPath == "" && isPrimaryModelArtifact(artifact) {
			modelPath = path
			continue
		}
		if isModelSupportArtifact(artifact) {
			support = append(support, modelSupportArtifact{Kind: artifact.Kind, Name: artifact.Name, Path: path})
		}
	}
	return modelPath, support
}

func isPrimaryModelArtifact(artifact ArtifactSpec) bool {
	kind := strings.ToLower(artifact.Kind)
	name := strings.ToLower(artifact.Name)
	return kind == "model_gguf" || strings.HasSuffix(name, ".gguf") && !isMMProjArtifact(artifact)
}

func isModelSupportArtifact(artifact ArtifactSpec) bool {
	kind := strings.ToLower(artifact.Kind)
	return strings.HasPrefix(kind, "model_") || isMMProjArtifact(artifact)
}

func isMMProjArtifact(artifact ArtifactSpec) bool {
	value := strings.ToLower(artifact.Kind + " " + artifact.Name)
	return strings.Contains(value, "mmproj")
}

func firstMMProjPath(support []modelSupportArtifact) string {
	for _, artifact := range support {
		value := strings.ToLower(artifact.Kind + " " + artifact.Name)
		if strings.Contains(value, "mmproj") {
			return artifact.Path
		}
	}
	return ""
}

func hasRuntimeArg(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}
