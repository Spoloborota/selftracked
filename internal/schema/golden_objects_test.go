package schema_test

// wantObjects is the schema's contents, written out rather than derived.
//
// The first version of these tests asked ddl.sql what it declared and then
// checked the database matched — which passes even when a trigger is deleted,
// because both sides shrink together. A reviewer proved that by removing
// epics_no_delete and watching the suite stay green. This list is the fixed
// point: losing an object from the DDL now fails here.
//
// Adding an object deliberately means adding it here, which is the review
// this list exists to force.
var wantObjects = []string{
	"artifacts",
	"artifacts_no_delete",
	"epic_artifacts",
	"epic_artifacts_no_delete",
	"epic_criteria",
	"epic_criteria_no_delete",
	"epics",
	"epics_close_gate",
	"epics_no_delete",
	"epics_transition",
	"events",
	"events_entity",
	"events_no_delete",
	"events_no_update",
	"meta",
	"path_dictionary",
	"path_dictionary_no_delete",
	"stories",
	"stories_no_delete",
	"stories_transition",
	"task_artifacts",
	"task_artifacts_no_delete",
	"task_links",
	"task_links_no_delete",
	"tasks",
	"tasks_inreview_exit_note",
	"tasks_no_delete",
	"tasks_transition",
	"v_backlog",
	"v_ready",
	"wip_one_per_epic",
	"worklog",
	"worklog_no_delete",
	"worklog_no_update",
	"worklog_seq_contiguous",
}
