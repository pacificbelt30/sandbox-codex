package workerpolicy

// QuarantinedLabel is a node-derived status. It cannot be cleared by restarting
// a worker or changing client configuration; create a new worker instead.
const QuarantinedLabel = "codex-dock.quarantined"
