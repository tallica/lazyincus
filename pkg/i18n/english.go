package i18n

// TranslationSet is a set of localised strings. For this MVP, English is the
// only supported language.
type TranslationSet struct {
	NotEnoughSpace                             string
	MainTitle                                  string
	GlobalTitle                                string
	Navigate                                   string
	Menu                                       string
	MenuTitle                                  string
	Execute                                    string
	Scroll                                     string
	Close                                      string
	Quit                                       string
	ErrorTitle                                 string
	NoViewMachingNewLineFocusedSwitchStatement string
	OpenConfig                                 string
	EditConfig                                 string
	ConfirmQuit                                string
	ErrorOccurred                              string
	CannotReachDaemonError                     string
	ConnectionLost                             string
	ConnectionLostTitle                        string
	CannotAttachStoppedInstanceError           string
	CannotAttachAppContainerError              string
	CannotExecStoppedInstanceError             string
	CannotAccessIncusSocketError               string
	CannotKillChildError                       string

	Cancel                       string
	Remove                       string
	HideStopped                  string
	HideUnmanagedNetworks        string
	PruneImages                  string
	PruneImagesTitle             string
	PruneCachedImages            string
	PruneUnusedImages            string
	ConfirmPruneImages           string
	NothingToPrune               string
	VolumeSizeUnknown            string
	UsedByNothing                string
	ImageUsersUnknown            string
	ShowUsers                    string
	InstancesUsing               string
	NothingUses                  string
	EditInEditor                 string
	CannotEditUnmanagedNetwork   string
	ACLsTitle                    string
	NoACLs                       string
	NoACLsUnmanaged              string
	ACLIngress                   string
	ACLEgress                    string
	ForwardsTitle                string
	NoForwards                   string
	NoForwardsUnmanaged          string
	CannotListForwards           string
	CannotSnapshotInstanceVolume string
	LeasesTitle                  string
	StateTitle                   string
	NoLeases                     string
	NoLeasesUnmanaged            string
	CannotListLeases             string
	CannotReadNetworkState       string
	ForceRemove                  string
	ForceStop                    string
	MustForceToRemove            string
	Confirm                      string
	Return                       string
	FocusMain                    string
	LcFilter                     string
	StopInstance                 string
	DeleteInstance               string
	ForceStopInstance            string
	RestartingStatus             string
	StartingStatus               string
	StoppingStatus               string
	PausingStatus                string
	ResumingStatus               string
	RemovingStatus               string
	ForceRemovingStatus          string
	Stop                         string
	Pause                        string
	Restart                      string
	Start                        string
	PreviousContext              string
	NextContext                  string
	Attach                       string
	ViewLogs                     string
	ExecShell                    string
	CopiedToClipboard            string
	Copy                         string
	CopyMenuTitle                string
	NothingToCopy                string
	CopyName                     string
	CopyAllAddresses             string
	CopyFingerprint              string
	CopyAlias                    string
	CopyPool                     string
	CopySnapshotRef              string
	InstanceTitle                string
	InstancesTitle               string
	ImagesTitle                  string
	ResourcesTitle               string
	PreviousList                 string
	NextList                     string
	NoImages                     string
	DeleteImage                  string
	NewSnapshot                  string
	RestoreSnapshot              string
	RestoreSnapshotShort         string
	DeleteSnapshot               string
	SnapshotNamePrompt           string
	SnapshotOptionsTitle         string
	SnapshotFocusName            string
	SnapshotExpiryField          string
	SnapshotStatefulField        string
	SnapshotSwitchFocusHint      string
	SnapshotChangeHint           string
	SnapshotSubmitHint           string
	SnapshotChangeValue          string
	SnapshotCreate               string
	SnapshottingStatus           string
	ToggleAllSnapshots           string
	AllSnapshotsLabel            string
	RestoringStatus              string
	LoadingStatus                string
	VolumesTitle                 string
	NoVolumes                    string
	DeleteVolume                 string
	CannotDeleteManagedVolume    string
	NetworksTitle                string
	ProfilesTitle                string
	ImagesShort                  string
	VolumesShort                 string
	NetworksShort                string
	ProfilesShort                string
	NoProfiles                   string
	DevicesTitle                 string
	NoDevices                    string
	DeleteProfile                string
	CannotDeleteDefaultProfile   string
	InProject                    string
	CannotDeleteInUse            string
	VolumeNamed                  string
	NetworkNamed                 string
	ProfileNamed                 string
	NoNetworks                   string
	DeleteNetwork                string

	ComposeTitle                  string
	StacksTitle                   string
	NoStacks                      string
	NoStackSelected               string
	AddStack                      string
	RemoveStack                   string
	AddStackPrompt                string
	AddStackHint                  string
	AddingStackStatus             string
	StackAlreadyListed            string
	StackNotComposeProject        string
	CannotRemoveLocalStack        string
	ConfirmRemoveStack            string
	StackNotRunning               string
	EndpointsTitle                string
	UsageTitle                    string
	DriftTitle                    string
	StackListedLocal              string
	StackListedSaved              string
	ServicesTitle                 string
	ServicesTitleProject          string
	NoServices                    string
	ServiceNotRunning             string
	ServiceNotInComposeFile       string
	ServiceMultipleInstances      string
	ServiceReplicaHeading         string
	ServiceInstanceHeading        string
	StandaloneInstancesTitle      string
	InfoTitle                     string
	ComposeTargetService          string
	ComposeTargetProject          string
	ComposeUp                     string
	ComposeUpPullRecreate         string
	ComposeDown                   string
	ComposeKill                   string
	ComposeBuild                  string
	ComposePull                   string
	ComposeLogs                   string
	ComposeStartWithDeps          string
	ComposeStartOnly              string
	ComposeDownMenuTitle          string
	ComposeStartMenuTitle         string
	ComposeServiceScoped          string
	ComposeDownOption             string
	ComposeDownWithVolumesOption  string
	ConfirmComposeDown            string
	ConfirmComposeDownWithVolumes string
	ConfirmComposeUpPullRecreate  string
	ConfirmComposeStop            string
	ConfirmComposeKill            string

	CannotDeleteUnmanagedNetwork string
	NoInstances                  string
	NoInstance                   string
	NoSnapshots                  string
	RemoveWithForce              string
	PressEnterToReturn           string
	ExitShellToReturn            string
	FilterList                   string
	SortInstancesByState         string

	LogsTitle                 string
	ConfigTitle               string
	EnvTitle                  string
	SnapshotsTitle            string
	TopTitle                  string
	CreditsTitle              string
	NothingToDisplay          string
	CannotDisplayEnvVariables string

	CannotListProcesses                string
	CannotListProcessesStoppedInstance string

	No  string
	Yes string

	LcNextScreenMode        string
	LcPrevScreenMode        string
	LcToggleExpandSidePanel string

	FilterPrompt string

	NextPanel string
	PrevPanel string

	// FocusPanel takes the panel's lowercased title, e.g. "instances".
	FocusPanel    string
	SwitchProject string
	ProjectsTitle string
	AllProjects   string
}

func englishSet() TranslationSet {
	return TranslationSet{
		RestartingStatus:    "restarting",
		StartingStatus:      "starting",
		StoppingStatus:      "stopping",
		PausingStatus:       "pausing",
		ResumingStatus:      "resuming",
		RemovingStatus:      "removing",
		ForceRemovingStatus: "stopping and deleting",

		NoViewMachingNewLineFocusedSwitchStatement: "No view matching newLineFocused switch statement",

		ErrorOccurred:                    "An error occurred! Please create an issue at https://github.com/tallica/lazyincus/issues",
		CannotReachDaemonError:           "Can't reach the Incus daemon: %v\nCheck that it's running, and that --remote, INCUS_REMOTE or the CLI's default-remote names the one you meant.",
		ConnectionLost:                   "Lost the connection to '%s'.\n\nStill trying - this closes itself once the daemon answers again.",
		ConnectionLostTitle:              "Connection lost",
		CannotAttachStoppedInstanceError: "You cannot attach to a stopped instance's console, you need to start it first (which you can do with the 'S' key)",
		CannotAttachAppContainerError:    "An OCI application container runs its image's entrypoint rather than an init system, so it has no console to attach to. Its output is the Logs tab ('m'), and 'E' gets you a shell.",
		CannotExecStoppedInstanceError:   "You cannot exec into a stopped instance, you need to start it first (which you can do with the 'S' key)",
		CannotAccessIncusSocketError:     "Can't access the incus socket.\nRun lazyincus as a user in the 'incus' group, or read https://linuxcontainers.org/incus/docs/main/installing/",
		CannotKillChildError:             "Waited three seconds for child process to stop. There may be an orphan process that continues to run on your system.",

		Confirm: "Confirm",

		Return:                       "return",
		FocusMain:                    "focus main panel",
		LcFilter:                     "filter list",
		Navigate:                     "navigate",
		Execute:                      "execute",
		Close:                        "close",
		Quit:                         "quit",
		Menu:                         "menu",
		MenuTitle:                    "Menu",
		Scroll:                       "scroll",
		OpenConfig:                   "open lazyincus config",
		EditConfig:                   "edit lazyincus config",
		Cancel:                       "cancel",
		Remove:                       "delete",
		HideStopped:                  "show/hide stopped instances",
		HideUnmanagedNetworks:        "show/hide host interfaces",
		PruneImages:                  "prune unused images",
		PruneImagesTitle:             "Prune images",
		PruneCachedImages:            "unused cached images (%d, %s)",
		PruneUnusedImages:            "every unused image (%d, %s)",
		ConfirmPruneImages:           "Delete these %d images (%s)? Nothing was created from them.\n\n%s",
		NothingToPrune:               "No images to prune.",
		VolumeSizeUnknown:            "unknown - the pool's driver doesn't report it",
		UsedByNothing:                "nothing",
		ImageUsersUnknown:            "unknown - the instances couldn't be listed",
		ShowUsers:                    "show the instances using it",
		InstancesUsing:               "Instances using %s",
		NothingUses:                  "No instance uses %s.",
		EditInEditor:                 "edit config in $EDITOR",
		CannotEditUnmanagedNetwork:   "Only networks Incus manages have a config to edit.",
		ACLsTitle:                    "ACLs",
		NoACLs:                       "No ACLs apply to this network, on it or on the NICs using it.",
		NoACLsUnmanaged:              "Only networks Incus manages take ACLs.",
		ACLIngress:                   "Ingress",
		ACLEgress:                    "Egress",
		ForwardsTitle:                "Forwards",
		NoForwards:                   "No forwards on this network.",
		NoForwardsUnmanaged:          "Only networks Incus manages have forwards.",
		CannotListForwards:           "Could not list the network's forwards.",
		CannotSnapshotInstanceVolume: "Only custom volumes take snapshots here: an instance's own volumes are snapshotted with the instance.",
		LeasesTitle:                  "Leases",
		StateTitle:                   "State",
		NoLeases:                     "No leases",
		NoLeasesUnmanaged:            "Only networks Incus manages hand out leases.",
		CannotListLeases:             "Could not list the network's leases.",
		CannotReadNetworkState:       "Could not read the network's state.",
		ForceRemove:                  "force delete",
		ForceStop:                    "force stop",
		MustForceToRemove:            "This instance is still running, so Incus refused to delete it. Stop it and delete it anyway?",
		Stop:                         "stop",
		Pause:                        "pause/resume",
		Restart:                      "restart",
		Start:                        "start",
		PreviousContext:              "previous tab",
		NextContext:                  "next tab",
		Attach:                       "attach to console",
		ViewLogs:                     "view logs",
		ExecShell:                    "exec shell",
		CopiedToClipboard:            "copied to clipboard:",
		Copy:                         "copy name, address…",
		CopyMenuTitle:                "Copy",
		NothingToCopy:                "Nothing to copy.",
		CopyName:                     "name",
		CopyAllAddresses:             "all addresses",
		CopyFingerprint:              "fingerprint",
		CopyAlias:                    "alias",
		CopyPool:                     "pool",
		CopySnapshotRef:              "owner/snapshot",
		FilterList:                   "filter list",
		SortInstancesByState:         "sort instances by state",

		GlobalTitle:                  "Global",
		MainTitle:                    "Main",
		InstanceTitle:                "Instance",
		InstancesTitle:               "Instances",
		ErrorTitle:                   "Error",
		LogsTitle:                    "Logs",
		ConfigTitle:                  "Config",
		EnvTitle:                     "Env",
		ImagesTitle:                  "Images",
		ResourcesTitle:               "Resources",
		PreviousList:                 "previous list",
		NextList:                     "next list",
		NoImages:                     "No images",
		DeleteImage:                  "Are you sure you want to delete image %s?",
		NewSnapshot:                  "new snapshot",
		ToggleAllSnapshots:           "show every instance's snapshots / the selected one's",
		AllSnapshotsLabel:            "all",
		RestoreSnapshot:              "Are you sure you want to restore %s to snapshot %s? Anything changed since is lost.",
		RestoreSnapshotShort:         "restore snapshot",
		DeleteSnapshot:               "Are you sure you want to delete snapshot %s of %s?",
		SnapshotNamePrompt:           "New snapshot of %s",
		SnapshotOptionsTitle:         "Options",
		SnapshotFocusName:            "back to name",
		SnapshotExpiryField:          "Expires in",
		SnapshotStatefulField:        "Stateful",
		SnapshotSwitchFocusHint:      "<tab> to toggle focus",
		SnapshotChangeHint:           "<← →> to change",
		SnapshotSubmitHint:           "<enter> or <ctrl+s> to create",
		SnapshotChangeValue:          "change value",
		SnapshotCreate:               "create",
		SnapshottingStatus:           "snapshotting",
		RestoringStatus:              "restoring",
		LoadingStatus:                "loading",
		VolumesTitle:                 "Volumes",
		NoVolumes:                    "No volumes",
		DeleteVolume:                 "Are you sure you want to delete volume %s?",
		NetworksTitle:                "Networks",
		ProfilesTitle:                "Profiles",
		ImagesShort:                  "Img",
		VolumesShort:                 "Vol",
		NetworksShort:                "Net",
		ProfilesShort:                "Prof",
		NoProfiles:                   "No profiles",
		DevicesTitle:                 "Devices",
		NoDevices:                    "The profile hands out no devices.",
		DeleteProfile:                "Are you sure you want to delete profile %s?",
		CannotDeleteDefaultProfile:   "A project's default profile can't be deleted: Incus keeps one in every project.",
		InProject:                    "%s in project %s",
		CannotDeleteInUse:            "%s is still in use (used by: %d), and Incus won't delete it until nothing uses it. u lists the instances using it.",
		VolumeNamed:                  "Volume %s",
		NetworkNamed:                 "Network %s",
		ProfileNamed:                 "Profile %s",
		NoNetworks:                   "No networks",
		DeleteNetwork:                "Are you sure you want to delete network %s?",
		CannotDeleteManagedVolume:    "Only custom volumes can be deleted. This one belongs to an instance or image, and goes away with it.",
		CannotDeleteUnmanagedNetwork: "Only managed networks can be deleted. This one is a host interface Incus doesn't control.",

		ComposeTitle:           "Compose",
		StacksTitle:            "Stacks",
		NoStacks:               "No stacks - press 'a' to add a compose project's directory.",
		NoStackSelected:        "No stack selected - add one to the Stacks panel with 'a'.",
		AddStack:               "add stack",
		RemoveStack:            "remove stack from the list",
		AddStackPrompt:         "Add stack: compose project directory",
		AddStackHint:           "enter to add · esc to cancel",
		AddingStackStatus:      "adding stack",
		StackAlreadyListed:     "%s is already listed.",
		StackNotComposeProject: "%s isn't a compose project incus-compose can read:\n\n%v",
		CannotRemoveLocalStack: "%s is the stack lazyincus started with, from the working directory or -P. It isn't saved, so there's nothing to remove.",
		ConfirmRemoveStack:     "Remove stack %s (%s) from the list? Nothing in it is stopped or deleted.",
		StackNotRunning:        "Nothing is running in this stack - press 'u' to bring it up.",
		EndpointsTitle:         "Endpoints",
		UsageTitle:             "Usage",
		DriftTitle:             "Drift",
		StackListedLocal:       "at startup (working directory or -P)",
		StackListedSaved:       "saved in state.yml",

		ServicesTitle:                 "Services",
		ServicesTitleProject:          "Services (%s)",
		NoServices:                    "No services",
		ServiceNotRunning:             "Nothing is running for this service - press 'u' to bring it up.",
		ServiceNotInComposeFile:       "This service is no longer in the compose file.",
		ServiceMultipleInstances:      "This service runs more than one replica. Select one in the list to see this tab for it.",
		ServiceReplicaHeading:         "Replica %d of %d · %s",
		ServiceInstanceHeading:        "Instance · %s",
		StandaloneInstancesTitle:      "Standalone Instances",
		InfoTitle:                     "Info",
		ComposeTargetService:          "service %s",
		ComposeTargetProject:          "project %s",
		ComposeServiceScoped:          "%s service",
		ComposeUp:                     "bring up",
		ComposeUpPullRecreate:         "pull & recreate",
		ComposeDown:                   "bring down",
		ComposeKill:                   "kill",
		ComposeBuild:                  "build",
		ComposePull:                   "pull",
		ComposeLogs:                   "logs --follow",
		ComposeStartWithDeps:          "start %s with %s",
		ComposeStartOnly:              "start %s only",
		ComposeDownMenuTitle:          "Down",
		ComposeStartMenuTitle:         "Start",
		ComposeDownOption:             "down",
		ComposeDownWithVolumesOption:  "down --volumes",
		ConfirmComposeDown:            "Are you sure you want to bring down %s?",
		ConfirmComposeDownWithVolumes: "Are you sure you want to bring down %s and delete its volumes?",
		ConfirmComposeUpPullRecreate:  "Are you sure you want to pull the latest images and recreate %s? Running instances will be replaced.",
		ConfirmComposeStop:            "Are you sure you want to stop %s?",
		ConfirmComposeKill:            "Are you sure you want to force stop %s?",

		SnapshotsTitle: "Snapshots",
		TopTitle:       "Top",
		CreditsTitle:   "About",

		NothingToDisplay:                   "Nothing to display",
		CannotListProcesses:                "Could not list processes.\n\n`ps` has to exist inside the instance, and a VM also\nneeds the Incus guest agent running.",
		CannotListProcessesStoppedInstance: "You cannot list the processes of a stopped instance (start it with the 'S' key)",
		CannotDisplayEnvVariables:          "Something went wrong while displaying instance details",

		NoInstances: "No instances",
		NoSnapshots: "No snapshots",
		NoInstance:  "No instance",

		ConfirmQuit:        "Are you sure you want to quit?",
		StopInstance:       "Are you sure you want to stop instance %s?",
		DeleteInstance:     "Are you sure you want to delete instance %s?",
		ForceStopInstance:  "Are you sure you want to force stop instance %s?",
		NotEnoughSpace:     "Not enough space to render panels",
		PressEnterToReturn: "Press enter to return to lazyincus (this prompt can be disabled in your config by setting `gui.returnImmediately: true`)",
		ExitShellToReturn:  "Exit the shell to return to lazyincus",

		No:  "no",
		Yes: "yes",

		LcNextScreenMode:        "next screen mode (normal/half/fullscreen)",
		LcPrevScreenMode:        "prev screen mode",
		LcToggleExpandSidePanel: "expand/collapse the focused side panel",

		FilterPrompt: "filter",

		NextPanel:     "next panel",
		PrevPanel:     "previous panel",
		FocusPanel:    "focus %s panel",
		SwitchProject: "switch project",
		ProjectsTitle: "Projects",
		AllProjects:   "all projects",
	}
}
