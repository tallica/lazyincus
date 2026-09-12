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
	ConnectionFailed                           string
	WaitingForInstanceInfo                     string
	CannotAttachStoppedInstanceError           string
	CannotExecStoppedInstanceError             string
	CannotAccessIncusSocketError               string
	CannotKillChildError                       string

	Donate                    string
	Cancel                    string
	Remove                    string
	HideStopped               string
	ForceRemove               string
	MustForceToRemove         string
	Confirm                   string
	Return                    string
	FocusMain                 string
	LcFilter                  string
	StopInstance              string
	DeleteInstance            string
	RestartingStatus          string
	StartingStatus            string
	StoppingStatus            string
	PausingStatus             string
	RemovingStatus            string
	ForceRemovingStatus       string
	Stop                      string
	Pause                     string
	Restart                   string
	Start                     string
	PreviousContext           string
	NextContext               string
	Attach                    string
	ViewLogs                  string
	ExecShell                 string
	CopyIPv4                  string
	CopiedToClipboard         string
	NoIPv4Address             string
	InstancesTitle            string
	ImagesTitle               string
	NoImages                  string
	DeleteImage               string
	NewSnapshot               string
	RestoreSnapshot           string
	RestoreSnapshotShort      string
	DeleteSnapshot            string
	SnapshotNamePrompt        string
	SnapshottingStatus        string
	RestoringStatus           string
	VolumesTitle              string
	NoVolumes                 string
	DeleteVolume              string
	CannotDeleteManagedVolume string
	NetworksTitle             string
	NoNetworks                string
	DeleteNetwork             string

	CannotDeleteUnmanagedNetwork string
	NoInstances                  string
	NoInstance                   string
	NoSnapshots                  string
	RemoveWithForce              string
	PressEnterToReturn           string
	ExitShellToReturn            string
	FilterList                   string
	SortInstancesByState         string

	StatsTitle                string
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

	LcNextScreenMode string
	LcPrevScreenMode string
	FilterPrompt     string

	NextPanel string
	PrevPanel string

	// FocusPanel takes the panel's lowercased title, e.g. "instances".
	FocusPanel    string
	SwitchProject string
	ProjectsTitle string
}

func englishSet() TranslationSet {
	return TranslationSet{
		RestartingStatus:    "restarting",
		StartingStatus:      "starting",
		StoppingStatus:      "stopping",
		PausingStatus:       "pausing",
		RemovingStatus:      "removing",
		ForceRemovingStatus: "stopping and deleting",

		NoViewMachingNewLineFocusedSwitchStatement: "No view matching newLineFocused switch statement",

		ErrorOccurred:                    "An error occurred! Please create an issue at https://github.com/tallica/lazyincus/issues",
		ConnectionFailed:                 "connection to the incus daemon failed. You may need to restart the incus daemon",
		WaitingForInstanceInfo:           "Cannot proceed until incus gives us more information about the instance. Please retry in a few moments.",
		CannotAttachStoppedInstanceError: "You cannot attach to a stopped instance's console, you need to start it first (which you can do with the 'S' key)",
		CannotExecStoppedInstanceError:   "You cannot exec into a stopped instance, you need to start it first (which you can do with the 'S' key)",
		CannotAccessIncusSocketError:     "Can't access the incus socket.\nRun lazyincus as a user in the 'incus' group, or read https://linuxcontainers.org/incus/docs/main/installing/",
		CannotKillChildError:             "Waited three seconds for child process to stop. There may be an orphan process that continues to run on your system.",

		Donate:  "Donate",
		Confirm: "Confirm",

		Return:               "return",
		FocusMain:            "focus main panel",
		LcFilter:             "filter list",
		Navigate:             "navigate",
		Execute:              "execute",
		Close:                "close",
		Quit:                 "quit",
		Menu:                 "menu",
		MenuTitle:            "Menu",
		Scroll:               "scroll",
		OpenConfig:           "open lazyincus config",
		EditConfig:           "edit lazyincus config",
		Cancel:               "cancel",
		Remove:               "delete",
		HideStopped:          "hide/show stopped instances",
		ForceRemove:          "force delete",
		MustForceToRemove:    "This instance is still running, so Incus refused to delete it. Stop it and delete it anyway?",
		Stop:                 "stop",
		Pause:                "pause/freeze",
		Restart:              "restart",
		Start:                "start",
		PreviousContext:      "previous tab",
		NextContext:          "next tab",
		Attach:               "attach to console",
		ViewLogs:             "view logs",
		ExecShell:            "exec shell",
		CopyIPv4:             "copy IPv4 address",
		CopiedToClipboard:    "copied to clipboard:",
		NoIPv4Address:        "This instance has no IPv4 address yet. It may still be starting up, or may not be running at all.",
		FilterList:           "filter list",
		SortInstancesByState: "sort instances by state",

		GlobalTitle:                  "Global",
		MainTitle:                    "Main",
		InstancesTitle:               "Instances",
		ErrorTitle:                   "Error",
		StatsTitle:                   "Stats",
		LogsTitle:                    "Logs",
		ConfigTitle:                  "Config",
		EnvTitle:                     "Env",
		ImagesTitle:                  "Images",
		NoImages:                     "No images",
		DeleteImage:                  "Are you sure you want to delete image %s?",
		NewSnapshot:                  "new snapshot",
		RestoreSnapshot:              "Are you sure you want to restore %s to snapshot %s? Anything changed since is lost.",
		RestoreSnapshotShort:         "restore snapshot",
		DeleteSnapshot:               "Are you sure you want to delete snapshot %s?",
		SnapshotNamePrompt:           "Name for the new snapshot of %s:",
		SnapshottingStatus:           "snapshotting",
		RestoringStatus:              "restoring",
		VolumesTitle:                 "Volumes",
		NoVolumes:                    "No volumes",
		DeleteVolume:                 "Are you sure you want to delete volume %s?",
		NetworksTitle:                "Networks",
		NoNetworks:                   "No networks",
		DeleteNetwork:                "Are you sure you want to delete network %s?",
		CannotDeleteManagedVolume:    "Only custom volumes can be deleted. This one belongs to an instance or image, and goes away with it.",
		CannotDeleteUnmanagedNetwork: "Only managed networks can be deleted. This one is a host interface Incus doesn't control.",
		SnapshotsTitle:               "Snapshots",
		TopTitle:                     "Top",
		CreditsTitle:                 "About",

		NothingToDisplay:                   "Nothing to display",
		CannotListProcesses:                "Could not list processes.\n\n`ps` has to exist inside the instance, and a VM also\nneeds the Incus guest agent running.",
		CannotListProcessesStoppedInstance: "You cannot list the processes of a stopped instance (start it with the 'S' key)",
		CannotDisplayEnvVariables:          "Something went wrong while displaying instance details",

		NoInstances: "No instances",
		NoSnapshots: "No snapshots",
		NoInstance:  "No instance",

		ConfirmQuit:        "Are you sure you want to quit?",
		StopInstance:       "Are you sure you want to stop this instance?",
		DeleteInstance:     "Are you sure you want to delete this instance?",
		NotEnoughSpace:     "Not enough space to render panels",
		PressEnterToReturn: "Press enter to return to lazyincus (this prompt can be disabled in your config by setting `gui.returnImmediately: true`)",
		ExitShellToReturn:  "Exit the shell to return to lazyincus",

		No:  "no",
		Yes: "yes",

		LcNextScreenMode: "next screen mode (normal/half/fullscreen)",
		LcPrevScreenMode: "prev screen mode",
		FilterPrompt:     "filter",

		NextPanel:     "next panel",
		PrevPanel:     "previous panel",
		FocusPanel:    "focus %s panel",
		SwitchProject: "switch project",
		ProjectsTitle: "Projects",
	}
}
