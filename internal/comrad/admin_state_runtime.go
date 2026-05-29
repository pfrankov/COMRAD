package comrad

import (
	"sort"
	"strings"
)

const runtimeSummaryAPIVersion = "comrad.local/v1"

func BuildRuntimeSummary(db Database) RuntimeSummary {
	adapters := runtimeAdaptersInUse(db)
	items := make([]RuntimeSummaryItem, 0, len(adapters))
	for _, adapter := range adapters {
		items = append(items, RuntimeSummaryItem{
			Metadata: ObjectMetadata{Name: adapter},
			Spec:     runtimeSummarySpec(adapter),
			Status:   runtimeSummaryStatus(db, adapter),
		})
	}
	return RuntimeSummary{APIVersion: runtimeSummaryAPIVersion, Kind: "RuntimeSummary", Items: items}
}

func runtimeAdaptersInUse(db Database) []string {
	seen := map[string]bool{"llama.cpp-metal": true}
	for _, profile := range db.Profiles {
		addRuntimeAdapter(seen, profile.RuntimeAdapter)
		for _, variant := range ProfileRuntimeVariants(profile) {
			addRuntimeAdapter(seen, variant.RuntimeAdapter)
		}
	}
	for _, node := range db.Nodes {
		for _, adapter := range node.RuntimeAdapters {
			addRuntimeAdapter(seen, adapter)
		}
	}
	for _, slot := range db.Slots {
		addRuntimeAdapter(seen, slot.RuntimeAdapter)
	}
	return sortedRuntimeAdapters(seen)
}

func addRuntimeAdapter(seen map[string]bool, adapter string) {
	adapter = strings.TrimSpace(adapter)
	if adapter != "" {
		seen[adapter] = true
	}
}

func sortedRuntimeAdapters(seen map[string]bool) []string {
	out := make([]string, 0, len(seen))
	for adapter := range seen {
		out = append(out, adapter)
	}
	sort.Strings(out)
	return out
}

func runtimeSummarySpec(adapter string) RuntimeSummarySpec {
	spec := RuntimeSummarySpec{
		Adapter:       adapter,
		ModelFormats:  []string{},
		TaskKinds:     []string{"llm.chat"},
		RuntimeBinary: RuntimeBinarySummary{Source: "worker-installed", Command: "worker-runtime"},
		ManagedArgs:   []string{},
	}
	if isLlamaCppAdapter(adapter) {
		spec.ModelFormats = []string{"gguf"}
		spec.RuntimeBinary.Command = "llama-server"
		spec.ManagedArgs = managedLlamaServerArgs()
	}
	return spec
}

func managedLlamaServerArgs() []string {
	return []string{
		"--host",
		"--port",
		"--model",
		"-m",
		"--mmproj",
		"--ctx-size",
		"-c",
		"--api-key",
		"--api-key-file",
		"--ssl-key-file",
		"--ssl-cert-file",
	}
}

func runtimeSummaryStatus(db Database, adapter string) RuntimeSummaryStatus {
	return RuntimeSummaryStatus{
		AvailableWorkers: availableWorkersForRuntime(db, adapter),
		ReadySlots:       readySlotsForRuntime(db, adapter),
	}
}

func availableWorkersForRuntime(db Database, adapter string) int {
	count := 0
	for _, node := range db.Nodes {
		if nodeRuntimeAvailable(node, adapter) {
			count++
		}
	}
	return count
}

func nodeRuntimeAvailable(node Node, adapter string) bool {
	return node.State == NodeStateOnline &&
		node.Approved &&
		!node.Quarantined &&
		Contains(node.RuntimeAdapters, adapter)
}

func readySlotsForRuntime(db Database, adapter string) int {
	count := 0
	for _, slot := range db.Slots {
		node := db.Nodes[slot.NodeID]
		if slotReadyForRuntime(node, slot, adapter) {
			count++
		}
	}
	return count
}

func slotReadyForRuntime(node Node, slot Slot, adapter string) bool {
	return nodeRuntimeAvailable(node, adapter) &&
		slot.RuntimeAdapter == adapter &&
		slot.State == SlotStateReady &&
		slot.AcceptsNew &&
		!slot.Quarantined
}
