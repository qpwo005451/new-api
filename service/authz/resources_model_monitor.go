package authz

const ResourceModelMonitor = "model_monitor"

var (
	ModelMonitorRead    = Permission{Resource: ResourceModelMonitor, Action: ActionRead}
	ModelMonitorOperate = Permission{Resource: ResourceModelMonitor, Action: ActionOperate}
	ModelMonitorWrite   = Permission{Resource: ResourceModelMonitor, Action: ActionWrite}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceModelMonitor,
		LabelKey: "Model availability monitor",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read model monitor",
				DescriptionKey: "View model availability summaries, details, history, and sanitized diagnostics.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Operate model monitor",
				DescriptionKey: "Queue a controlled model availability monitoring run.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Configure model monitor",
				DescriptionKey: "Edit monitored sites, channel ownership, targets, weights, and scheduling settings.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
		},
	})
}
