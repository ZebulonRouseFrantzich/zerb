"""Bounded, source-catalogued diagnostics; never retain arbitrary tool output."""

from collections import deque
import json
import re
from pathlib import PurePosixPath
from types import MappingProxyType

MAX_LINE = 65536
MAX_EVENTS = 4096
MAX_CATALOG_BYTES = 16 * 1024 * 1024
MAX_CATALOG_FILES = 4096
MAX_SOURCE_BYTES = 1024 * 1024
ROLES = frozenset(("F", "M", "O", "S", "G"))
MODES = frozenset(("held", "foreign-sender", "wrong-parent", "wrong-nonce",
    "replayed-phase", "substituted-backup", "interrupted-S", "interrupted-G",
    "owner-loss", "queued-owner-ack", "marker-fd"))
# Keep this diagnostic-only vocabulary aligned with the reviewed producer.
EVENTS = frozenset(("send", "received", "birth", "wait", "retained-exit", "refusal",
    "error-send", "error-receive", "error-live-before", "error-live-after",
    "error-wait", "error-retained-exit", "error-birth"))
PHASES = frozenset(("startup", "cleanup", "status", "unknown", "hello", "scope", "scoped", "birth",
    "birth-refused", "birth-interrupted", "parent-scope", "parent-scoped",
    "register", "registered", "retire", "retired", "arm-request", "arm",
    "arm-refused", "stop", "stop-G", "child-status", "refused",
    "parent-register", "parent-registered"))


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate diagnostic key")
        result[key] = value
    return result


def source_catalog(sources):
    """Freeze identifiers from bytes captured by the pre-execution source scan."""
    if len(sources) > MAX_CATALOG_FILES or sum(len(text) for text in sources.values()) > MAX_CATALOG_BYTES:
        raise ValueError("diagnostic catalog bound")
    catalog = {}
    modules = {}
    for name, raw in sources.items():
        if len(raw) > MAX_SOURCE_BYTES or len(name) > 1024 or not re.fullmatch(r"[A-Za-z0-9_./-]+", name):
            raise ValueError("diagnostic source bound")
        path = PurePosixPath(name)
        if path.is_absolute() or any(part in ("..", ".git") for part in path.parts):
            raise ValueError("diagnostic source path")
        if path.name == "go.mod":
            module = re.search(r"^module ([A-Za-z0-9./_-]{1,256})$", raw.decode(), re.MULTILINE)
            if module:
                modules[path.parent] = module[1]
    for name, raw in sources.items():
        path = PurePosixPath(name)
        if path.suffix != ".go":
            continue
        root = next((parent for parent in path.parents if parent in modules), None)
        if root is None:
            continue
        relative = path.relative_to(root)
        package = modules[root] + ("/" + str(relative.parent) if relative.parent.parts else "")
        if len(package) > 1024:
            raise ValueError("diagnostic package bound")
        text = raw.decode()
        entry = catalog.setdefault(package, {"tests": set(), "files": {}})
        entry["tests"].update(re.findall(r"^func (Test[A-Za-z0-9_]{0,252})\(", text, re.MULTILINE))
        entry["files"].setdefault(path.name, []).append((name, len(text.splitlines())))
    return MappingProxyType({name: MappingProxyType({"tests": frozenset(row["tests"]),
        "files": MappingProxyType({file: tuple(locations) for file, locations in row["files"].items()})})
        for name, row in catalog.items()})


def assembly_record(value):
    fields = {"role", "mode", "sequence", "event", "phase", "subject"}
    if not isinstance(value, dict) or set(value) not in (fields, fields | {"raw_status"}):
        return False
    if any(not isinstance(value[field], str) for field in ("role", "mode", "event", "phase", "subject")):
        return False
    if (value["role"] not in ROLES | {"harness", "fixture"} or value["mode"] not in MODES or
            value["event"] not in EVENTS or value["phase"] not in PHASES or
            value["subject"] not in ROLES | {"unknown"} or
            type(value["sequence"]) is not int or not 0 < value["sequence"] < 2**64):
        return False
    has_status = "raw_status" in value
    return (has_status == (value["event"] in ("wait", "retained-exit")) and
            (not has_status or type(value["raw_status"]) is int and 0 <= value["raw_status"] <= 65535))


def failure_diagnostics(log, catalog):
    """Locations and typed records are observations, never authentication proof."""
    events = deque(maxlen=MAX_EVENTS)
    failures = []
    locations = []
    dropped, ordinal = 0, 0
    log.seek(0)
    while True:
        raw = log.readline(MAX_LINE + 1)
        if not raw:
            break
        ordinal += 1
        if len(raw) > MAX_LINE:
            dropped += 1
            while raw and not raw.endswith(b"\n"):
                raw = log.readline(MAX_LINE + 1)
            continue
        try:
            record = json.loads(raw, object_pairs_hook=unique_object)
        except (ValueError, UnicodeError, RecursionError):
            dropped += 1
            continue
        if not isinstance(record, dict):
            dropped += 1
            continue
        package = record.get("Package")
        entry = catalog.get(package) if isinstance(package, str) else None
        if entry is None:
            dropped += 1
            continue
        test = record.get("Test", "")
        if not isinstance(test, str):
            dropped += 1
            continue
        test = test.split("/", 1)[0]
        if test and test not in entry["tests"]:
            dropped += 1
            continue
        base = {"stream_ordinal": ordinal, "package": package, "test": test}
        action = record.get("Action")
        if action in ("run", "pass", "fail", "skip"):
            event = {**base, "kind": "test-" + action}
            if len(events) == MAX_EVENTS:
                dropped += 1
            events.append(event)
            if action == "fail":
                if len(failures) < MAX_EVENTS:
                    associated = [row["stream_ordinal"] for row in reversed(locations)
                                  if row["package"] == package and row["test"] == test][:8]
                    window = [row for row in reversed(events) if row["package"] == package and
                              row["test"] == test and row["kind"] == "assembly-observation"][:16]
                    failures.append({**event, "location_ordinals": list(reversed(associated)),
                                     "recent_assembly_observations": list(reversed(window))})
                else:
                    dropped += 1
        elif action == "output" and isinstance(record.get("Output"), str):
            for line in record["Output"].splitlines():
                location = re.match(r"\s*([A-Za-z0-9_]+\.go):([0-9]{1,7}):", line)
                value = None
                if location:
                    files = entry["files"].get(location[1], [])
                    if len(files) == 1 and 0 < int(location[2]) <= files[0][1]:
                        value = {**base, "kind": "source-location", "file": files[0][0], "line": int(location[2])}
                        if len(locations) < MAX_EVENTS:
                            locations.append(value)
                        else:
                            dropped += 1
                    else:
                        dropped += 1
                elif line.lstrip().startswith("ZERB_ASSEMBLY_TRACE "):
                    try:
                        trace = json.loads(line.lstrip().removeprefix("ZERB_ASSEMBLY_TRACE "), object_pairs_hook=unique_object)
                    except (ValueError, RecursionError):
                        trace = None
                    if len(line.lstrip().encode()) <= 512 and assembly_record(trace):
                        value = {**base, "kind": "assembly-observation", "observation": trace}
                    else:
                        dropped += 1
                # Arbitrary formatted output is deliberately never copied.
                if value is not None:
                    if len(events) == MAX_EVENTS:
                        dropped += 1
                    events.append(value)
        elif action not in ("start", "pause", "cont", "bench"):
            dropped += 1
    located = bool(locations)
    return {"schema": "zerb-safe-failure-diagnostics-v1", "status": "incomplete" if dropped or not failures or not located else "catalogued",
            "dropped_records": dropped, "failures": failures, "locations": locations, "observations": list(events),
            "ordering": "collector-arrival and producer-local sequence only; not a global causal order",
            "coverage": "source locations and instrumented held-assembly statuses only; arbitrary output omitted"}
