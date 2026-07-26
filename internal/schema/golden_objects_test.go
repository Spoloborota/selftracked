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
	"task_links",
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

// wantDDLDigest is SHA-256 over `schema.DDL()` — the compiled-in ddl.sql,
// every byte of it, comments and blank lines included.
//
// The DDL's BYTE-stability is the load-bearing property, not merely its
// shape: §8.5 has `load` compare a dump's DDL block against this text byte
// for byte, so any edit at all — a column, a trigger body, a rewrapped
// comment — makes every existing dump unloadable and moves the schema
// version. A digest is the only pin that covers that whole surface, which is
// why it is here as well as `wantColumns`: a column list says nothing about
// a trigger, and a name list (`wantObjects`) says nothing about either.
//
// Updating this constant is how a schema change is declared deliberate. If
// it moved and you did not mean to change the schema, the change is the bug.
const wantDDLDigest = "2f292df0675264c9f64e59a36e02ba485cbda55739911df0eab0b7a70552779b"

// wantColumns is every table's column list, in declaration order, written
// out rather than derived — the same fixed-point discipline as wantObjects,
// one level down.
//
// It is kept ALONGSIDE the digest and not instead of it, because the two
// fail differently and both failures are wanted. The digest proves the whole
// text is unchanged but can only report that something is; this list names
// the table and the column, which is what a reader needs to tell a deliberate
// migration from a stray edit. It is also read from a live database rather
// than parsed out of the DDL, so it re-proves that the text and the database
// SQLite builds from it still agree.
var wantColumns = map[string][]string{
	"artifacts":       {"id", "class", "scope", "relpath", "archived", "note"},
	"epic_artifacts":  {"epic", "artifact", "role", "note"},
	"epic_criteria":   {"epic", "seq", "criterion", "met", "evidence"},
	"epics":           {"slug", "goal", "status", "status_note", "close_sweep", "created_at"},
	"events":          {"seq", "at", "entity", "event", "detail"},
	"meta":            {"key", "value"},
	"path_dictionary": {"class", "scope", "root", "ephemeral", "note"},
	"stories":         {"epic", "id", "title", "status", "dod", "consumes", "produces", "blocked"},
	"task_artifacts":  {"task", "artifact", "role", "note"},
	"task_links":      {"from_task", "to_task", "type", "note"},
	"tasks": {
		"id", "title", "status", "status_note", "parked",
		"dup_of", "epic", "created_at", "updated_at",
	},
	"worklog": {
		"epic", "seq", "story", "date", "state",
		"commits", "gate", "review", "corrects", "note",
	},
}
