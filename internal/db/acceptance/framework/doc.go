// Package framework implements the core orchestration infrastructure, telemetry metrics,
// resource management and session lifecycles of the PebbleDB Acceptance Testing Framework (ATF).
//
// Subpackages:
//   - config: Configuration loader with priority override cascading.
//   - errors: Custom typed framework runtime and validation error wrappers.
//   - eventbus: Asynchronous, thread-safe local event bus with context propagation.
//   - interfaces: Core contracts mapping to scenario, dataset, and verification systems.
//   - logging: Thread-safe, context-injected structured logs.
//   - registry: Dynamic scenario configuration cache and duplicate checker.
//   - resource: CPU, RAM, file descriptor semaphores and sandbox path allocator.
//   - runner: Step orchestration skeleton and session state transition.
//   - scheduler: Queue ordering, priority sorting, and topological dependency resolver.
//   - session: Campaign/Scenario tracking state machines.
//   - telemetry: Metrics, duration recorders, and campaign aggregators.
//   - types: Base structs, state/status/priority enums.
//   - util: Safe file copy and checksum helpers.
//
// Package Dependency Rules (Downwards flow only):
//
//   API / CAMPAIGN Layer:
//     interfaces, types, errors, logging
//       │ (Leaves - import nothing else)
//       ▼
//   INFRASTRUCTURE Layer:
//     eventbus, telemetry, resource, session, config, registry, util
//       │ (Internal engines - import only leaves)
//       ▼
//   SCHEDULER / RUNNER Layer:
//     scheduler, runner
//       (Orchestrators - import infrastructure and leaf packages)
package framework
