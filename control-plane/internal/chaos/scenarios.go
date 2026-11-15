package chaos

// Scenario enumerates S20 failure injections for runbooks.
type Scenario string

const (
	ScenarioNodeDown       Scenario = "node_down"
	ScenarioPartition      Scenario = "network_partition"
	ScenarioDiskFull       Scenario = "disk_full"
	ScenarioControlRestart Scenario = "control_plane_restart"
)

// All returns the baseline chaos catalog.
func All() []Scenario {
	return []Scenario{ScenarioNodeDown, ScenarioPartition, ScenarioDiskFull, ScenarioControlRestart}
}
