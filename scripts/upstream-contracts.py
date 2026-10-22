#!/usr/bin/env python3
"""Read-only upstream contract capture and conservative, deterministic comparison.

No upstream module is imported and no login, inference or token refresh is run.
CLI execution is limited to help/version/schema export with a fresh HOME and an
environment allowlist. A successful report means no *monitored static* drift.
"""
import argparse
import ast
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
from urllib.parse import urlsplit
from urllib.request import Request, urlopen


ROOT = Path(__file__).resolve().parent.parent
DEFAULT_MANIFEST = ROOT / "compat/upstream/manifest.json"
LIMIT = 16 * 1024 * 1024
MISSING = object()
LEVELS = {"info": 0, "review": 1, "breaking": 2, "error": 3}
DOC_KEYS = {"description", "title", "summary", "example", "examples",
            "externalDocs", "$comment", "x-oaiMeta", "x-stainless-naming"}
NAMED_MAPS = {"properties", "patternProperties", "definitions", "$defs",
              "schemas", "paths", "responses", "content", "securitySchemes"}


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":"),
                                     ensure_ascii=False).encode()).hexdigest()


def read_json(path):
    return json.loads(Path(path).read_text())


def write_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n")


def pointer(document, ref):
    if ref == "#":
        return document
    if not ref.startswith("#/"):
        raise ValueError("external schema references are unsupported: " + ref)
    value = document
    for part in ref[2:].split("/"):
        key = part.replace("~1", "/").replace("~0", "~")
        value = value[int(key)] if isinstance(value, list) else value[key]
    return value


def clean(value, named=False):
    """Ignore prose, preserving properties literally named e.g. title/example."""
    if isinstance(value, dict):
        return {k: clean(v, k in NAMED_MAPS) for k, v in value.items()
                if named or k not in DOC_KEYS}
    if isinstance(value, list):
        return [clean(v) for v in value]
    return value


def schema_bundle(document, roots):
    refs = {}

    def visit(value):
        if isinstance(value, dict):
            ref = value.get("$ref")
            if ref is not None and ref not in refs:
                refs[ref] = clean(pointer(document, ref))
                visit(refs[ref])
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)

    roots = {key: clean(value) for key, value in roots.items()}
    visit(roots)
    return {"roots": roots, "refs": refs}


def python_types(text):
    """Parse, never execute, published SDK annotations and type declarations."""
    tree = ast.parse(text)
    declarations = {}
    imports = []

    def node_value(node):
        # ast.dump formatting/empty-field defaults vary across Python versions.
        if isinstance(node, ast.AST):
            return {"node": type(node).__name__, **{key: node_value(value)
                    for key, value in ast.iter_fields(node) if value is not None and value != []}}
        if isinstance(node, list):
            return [node_value(value) for value in node]
        return node

    for node in tree.body:
        if isinstance(node, (ast.Import, ast.ImportFrom)):
            imports.append(node_value(node))
        elif isinstance(node, ast.ClassDef):
            # Method implementations/docstrings are not the SDK wire contract.
            fields = {}
            for member in node.body:
                if isinstance(member, ast.AnnAssign) and isinstance(member.target, ast.Name):
                    fields[member.target.id] = node_value(member)
            declarations[node.name] = {
                "bases": [node_value(v) for v in node.bases],
                "keywords": [node_value(v) for v in node.keywords],
                "fields": fields,
            }
        elif isinstance(node, (ast.Assign, ast.AnnAssign)):
            # Includes type aliases/unions. Their meaning is reviewed on change.
            targets = node.targets if isinstance(node, ast.Assign) else [node.target]
            for target in targets:
                if isinstance(target, ast.Name) and not target.id.startswith("_"):
                    declarations[target.id] = node_value(node)
        elif hasattr(ast, "TypeAlias") and isinstance(node, ast.TypeAlias):
            declarations[node.name.id] = node_value(node)
    if not declarations:
        raise ValueError("no SDK type declarations found")
    return {"imports": sorted(imports, key=digest), "declarations": declarations}


def fetch(url):
    parts = urlsplit(url)
    if parts.scheme != "https" or not parts.hostname or parts.username or parts.password or parts.fragment:
        raise ValueError("upstream sources must be credential-free HTTPS URLs")
    request = Request(url, headers={"User-Agent": "sandbox-codex-contract-check/1"})
    with urlopen(request, timeout=45) as response:
        if urlsplit(response.url).scheme != "https":
            raise ValueError("upstream redirected away from HTTPS")
        data = response.read(LIMIT + 1)
    if len(data) > LIMIT:
        raise ValueError("upstream artifact exceeds 16 MiB")
    return data


def load_document(data):
    try:
        return json.loads(data)
    except json.JSONDecodeError:
        import yaml  # Only OpenAPI YAML needs the optional development dependency.
        document = yaml.safe_load(data)
        if not isinstance(document, dict):
            raise ValueError("expected a schema object")
        return document


def cli_run(binary, arguments, home):
    # Do not inherit API keys, login files, SSH agents, cloud selectors or hooks.
    allowed = ("PATH", "SYSTEMROOT", "WINDIR", "TMPDIR", "SSL_CERT_FILE",
               "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS")
    env = {name: os.environ[name] for name in allowed if name in os.environ}
    env.update(HOME=str(home), CODEX_HOME=str(home / "codex"),
               CLAUDE_CONFIG_DIR=str(home / "claude"), NO_COLOR="1", TERM="dumb",
               DISABLE_AUTOUPDATER="1", CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1")
    for directory in (home / "codex", home / "claude"):
        directory.mkdir(exist_ok=True)
    result = subprocess.run([str(Path(binary).resolve()), *arguments], cwd=home,
                            env=env, capture_output=True, timeout=90, check=False)
    if result.returncode:
        # Raw stderr can contain local details. Store only deterministic evidence.
        raise ValueError("CLI command failed: " + " ".join(arguments) +
                         " (exit " + str(result.returncode) + ")")
    if len(result.stdout) > LIMIT:
        raise ValueError("CLI output exceeds 16 MiB")
    return result.stdout.decode("utf-8")


def help_flags(text):
    flags = {}
    current = []
    for line in text.splitlines():
        if re.match(r"^\s+(-[A-Za-z?],?\s+)?--[A-Za-z]", line):
            current = re.findall(r"(?<![\w-])--[A-Za-z][\w-]*", line)
            for flag in current:
                flags[flag] = " ".join(line.split())
        elif current and line[:1].isspace() and line.strip():
            for flag in current:
                flags[flag] += " " + " ".join(line.split())
        else:
            current = []
    if not flags:
        raise ValueError("no CLI flags found; help format may have changed")
    return flags


def resolve_source(url, revisions):
    """Resolve floating GitHub refs once per capture; record reproducible URLs."""
    parts = urlsplit(url)
    if parts.hostname != "raw.githubusercontent.com":
        return url
    owner, repo, ref, path = parts.path.lstrip("/").split("/", 3)
    if re.fullmatch(r"[0-9a-f]{40}", ref):
        return url
    key = (owner, repo, ref)
    if key not in revisions:
        metadata = json.loads(fetch("https://api.github.com/repos/" + owner + "/" + repo + "/commits/" + ref))
        revision = metadata["sha"]
        if not re.fullmatch(r"[0-9a-f]{40}", revision):
            raise ValueError("invalid upstream Git revision")
        revisions[key] = revision
    return "https://raw.githubusercontent.com/" + owner + "/" + repo + "/" + revisions[key] + "/" + path


def collect_source(source, binaries, home, revisions):
    kind = source["kind"]
    result = {"kind": kind, "usage": source.get("usage", "required")}
    if kind in ("cli", "codex-schema"):
        binary = binaries.get(source["binary"])
        if not binary:
            raise ValueError("missing --" + source["binary"] + " binary")
        result["version"] = cli_run(binary, ["--version"], home).strip()
        if kind == "cli":
            commands = {}
            for command in source["commands"]:
                flags = help_flags(cli_run(binary, [*command["argv"], "--help"], home))
                commands[command["name"]] = flags
            result["data"] = commands
        else:
            output = home / "schema"
            cli_run(binary, ["app-server", "generate-json-schema", "--out", str(output)], home)
            roots = {}
            for filename in source["files"]:
                matches = list(output.rglob(filename))
                if len(matches) != 1:
                    raise ValueError("expected exactly one generated " + filename)
                document = read_json(matches[0])
                bundle = schema_bundle(document, {"request:" + filename: document})
                # Each generated file has its own definitions namespace.
                roots[filename] = bundle
            result["data"] = roots
        return result
    url = resolve_source(source["url"], revisions)
    data = fetch(url)
    result["source"] = url
    result["tracking_source"] = source["url"]
    result["artifact_sha256"] = hashlib.sha256(data).hexdigest()
    if kind == "schema":
        document = load_document(data)
        result["data"] = schema_bundle(document, {ref: pointer(document, ref) for ref in source["roots"]})
    elif kind == "openapi":
        document = load_document(data)
        if not str(document.get("openapi", "")).startswith("3."):
            raise ValueError("only OpenAPI 3.x is supported")
        roots = {}
        for method, path in source["operations"]:
            operation = document.get("paths", {}).get(path, {}).get(method.lower())
            if operation is None:
                # Keep an explicit absence so removal is compared as a contract change.
                roots[method + " " + path] = None
                continue
            path_item = document["paths"][path]
            roots[method + " " + path] = {
                "operation": operation,
                "pathParameters": path_item.get("parameters", []),
                "servers": operation.get("servers", path_item.get("servers", document.get("servers", []))),
                "security": operation.get("security", document.get("security", [])),
            }
        roots["securitySchemes"] = document.get("components", {}).get("securitySchemes", {})
        result["data"] = schema_bundle(document, roots)
    elif kind == "python-types":
        result["data"] = python_types(data.decode("utf-8"))
    elif kind == "text":
        text = data.decode("utf-8").replace("\r\n", "\n")
        if source.get("match_lines"):
            text = "\n".join(line for line in text.splitlines() if re.search(source["match_lines"], line))
            if not text:
                raise ValueError("no monitored authentication release notes found")
        result["data"] = {"sha256": hashlib.sha256(text.encode()).hexdigest()}
    else:
        raise ValueError("unsupported source kind: " + kind)
    return result


def validate_manifest(manifest):
    if manifest.get("schema") != 1 or not manifest.get("sources"):
        raise ValueError("manifest must have schema 1 and nonempty sources")
    ids = [source["id"] for source in manifest["sources"]]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate source id")
    for source in manifest["sources"]:
        if source.get("usage", "required") not in ("required", "advisory"):
            raise ValueError("invalid usage for " + source["id"])


def validate_bundle(bundle, allow_missing=False):
    if not isinstance(bundle, dict) or not isinstance(bundle.get("roots"), dict) or not bundle["roots"]:
        raise ValueError("missing schema roots")
    if not isinstance(bundle.get("refs"), dict):
        raise ValueError("missing schema reference map")
    for value in bundle["roots"].values():
        if not isinstance(value, (dict, bool)) and not (allow_missing and value is None):
            raise ValueError("schema roots must be objects or boolean schemas")
    if not all(isinstance(value, (dict, bool)) for value in bundle["refs"].values()):
        raise ValueError("invalid referenced schema")

    def visit(value):
        if isinstance(value, dict):
            if "$ref" in value and value["$ref"] not in bundle["refs"]:
                raise ValueError("unresolved schema reference")
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)
    visit(bundle)


def validate_source(spec, snapshot):
    kind, data = spec["kind"], snapshot["data"]
    if kind in ("schema", "openapi"):
        validate_bundle(data, allow_missing=kind == "openapi")
        expected = set(spec["roots"]) if kind == "schema" else {
            method + " " + path for method, path in spec["operations"]} | {"securitySchemes"}
        if set(data["roots"]) != expected:
            raise ValueError("schema roots do not match manifest")
    elif kind == "codex-schema":
        if set(data) != set(spec["files"]):
            raise ValueError("generated schema files do not match manifest")
        for value in data.values():
            validate_bundle(value)
    elif kind == "cli":
        if set(data) != {command["name"] for command in spec["commands"]}:
            raise ValueError("CLI commands do not match manifest")
        if not snapshot.get("version") or not all(isinstance(flags, dict) and flags for flags in data.values()):
            raise ValueError("missing CLI version or flags")
    elif kind == "python-types":
        if not isinstance(data.get("declarations"), dict) or not data["declarations"] or not isinstance(data.get("imports"), list):
            raise ValueError("missing SDK declarations or imports")
    elif kind == "text":
        if not re.fullmatch(r"[0-9a-f]{64}", data.get("sha256", "")):
            raise ValueError("missing authentication source fingerprint")
    else:
        raise ValueError("unknown source kind")


def capture(manifest, binaries):
    validate_manifest(manifest)
    snapshot = {"schema": 1, "manifest_sha256": digest(manifest), "sources": {}, "errors": [],
                "captured_at": datetime.now(timezone.utc).isoformat()}
    revisions = {}
    for source in manifest["sources"]:
        try:
            with tempfile.TemporaryDirectory(prefix="cxd-contract-") as temporary:
                value = collect_source(source, binaries, Path(temporary), revisions)
                validate_source(source, value)
                snapshot["sources"][source["id"]] = value
        except Exception as error:
            # No artifact body / CLI environment / credential is logged.
            snapshot["errors"].append({"source": source["id"], "message": type(error).__name__ + ": " + str(error)})
    return snapshot


def finding(findings, source, level, rule, path, message):
    findings.append({"source": source, "level": level, "rule": rule,
                     "path": path, "message": message})


def schema_changes(old, new, source, findings):
    visited = set()
    refs_equal = old["refs"] == new["refs"]

    def has_ref(value):
        if isinstance(value, dict):
            return "$ref" in value or any(has_ref(v) for v in value.values())
        return isinstance(value, list) and any(has_ref(v) for v in value)

    def walk(a, b, path, role="request"):
        if a == b and (refs_equal or not has_ref(a)):
            return
        if (isinstance(a, dict) and "$ref" in a) or (isinstance(b, dict) and "$ref" in b):
            ar = a.get("$ref") if isinstance(a, dict) else None
            br = b.get("$ref") if isinstance(b, dict) else None
            a_siblings = {k: v for k, v in a.items() if k != "$ref"} if ar else {}
            b_siblings = {k: v for k, v in b.items() if k != "$ref"} if br else {}
            identity = (ar, br, role, digest(a_siblings), digest(b_siblings))
            if ar and br and identity in visited:
                return
            if ar and br:
                visited.add(identity)
            # JSON Schema 2020-12 permits meaningful siblings alongside $ref.
            if ar:
                target = old["refs"][ar]
                a = {"allOf": [target, a_siblings]} if a_siblings else target
            if br:
                target = new["refs"][br]
                b = {"allOf": [target, b_siblings]} if b_siblings else target
            walk(a, b, path, role)
            return
        if a == b and not isinstance(a, (dict, list)):
            return
        if role == "security":
            finding(findings, source, "review", "security-contract-changed", path,
                    "Authentication, destination or security contract changed")
            return
        if isinstance(a, dict) and isinstance(b, dict):
            for key in sorted(a.keys() | b.keys()):
                av, bv = a.get(key, MISSING), b.get(key, MISSING)
                location = path + "/" + key.replace("~", "~0").replace("/", "~1")
                next_role = "response" if key == "responses" else role
                if key in ("requestBody", "parameters", "pathParameters"):
                    next_role = "request"
                if key in ("security", "securitySchemes", "servers"):
                    next_role = "security"
                if av == bv and refs_equal:
                    continue
                if key == "required" and isinstance(av, list) and isinstance(bv, list):
                    removed, added = set(av) - set(bv), set(bv) - set(av)
                    breaking = added if role == "request" else removed
                    if removed or added:
                        finding(findings, source, "breaking" if breaking else "info",
                                "required-fields-changed", location,
                                "Required fields changed: added=" + str(sorted(added)) + "; removed=" + str(sorted(removed)))
                elif key == "required" and isinstance(av, bool) and isinstance(bv, bool) and av != bv:
                    finding(findings, source, "breaking" if role == "request" and bv else "review",
                            "required-body-changed", location, "Body/parameter requirement changed")
                elif key == "enum" and isinstance(av, list) and isinstance(bv, list):
                    removed = {digest(v) for v in av} - {digest(v) for v in bv}
                    added = {digest(v) for v in bv} - {digest(v) for v in av}
                    if removed or added:
                        breaking = removed if role == "request" else added
                        finding(findings, source, "breaking" if breaking else "info", "enum-values-changed",
                                location, "Enum values changed (direction: " + role + ")")
                elif av is MISSING or bv is MISSING:
                    # Adding/removing validation constraints can narrow a contract;
                    # unknown schema keywords are never assumed compatible.
                    level = "review"
                    if key in ("required", "type", "enum", "const"):
                        if key == "required":
                            values = bv if av is MISSING else av
                            level = ("breaking" if bool(values) and
                                     ((role == "request") == (av is MISSING)) else "info")
                    if key == "deprecated":
                        level = "info"
                    finding(findings, source, level, "schema-key-changed", location,
                            "Schema key " + ("added" if av is MISSING else "removed"))
                elif key in ("type", "const") and av != bv:
                    finding(findings, source, "breaking", "schema-type-changed", location, "Type or constant changed")
                elif key == "properties" and isinstance(av, dict) and isinstance(bv, dict):
                    for prop in sorted(av.keys() | bv.keys()):
                        pp = location + "/" + prop.replace("~", "~0").replace("/", "~1")
                        if prop not in bv:
                            finding(findings, source, "breaking", "property-removed", pp, "Previously declared property removed")
                        elif prop not in av:
                            # A required request property is caught by required above.
                            finding(findings, source, "info", "property-added", pp, "Property added")
                        else:
                            walk(av[prop], bv[prop], pp, role)
                else:
                    walk(av, bv, location, next_role)
        elif isinstance(a, list) and isinstance(b, list) and a == b:
            for index, (av, bv) in enumerate(zip(a, b)):
                walk(av, bv, path + "/" + str(index), role)
        else:
            # Composition, defaults, bounds, flags and array changes need review.
            finding(findings, source, "review", "schema-value-changed", path, "Schema value changed")

    for key in sorted(old["roots"].keys() | new["roots"].keys()):
        a, b = old["roots"].get(key, MISSING), new["roots"].get(key, MISSING)
        if b is MISSING or b is None and a is not None and a is not MISSING:
            finding(findings, source, "breaking", "contract-removed", key, "Monitored operation/schema removed")
        elif a is MISSING or a is None and b is not None:
            finding(findings, source, "info", "contract-added", key, "Monitored operation/schema added")
        elif a is None and b is None:
            finding(findings, source, "review", "contract-unavailable", key, "Monitored operation absent in both snapshots")
        else:
            walk(a, b, key, "security" if key == "securitySchemes" else "request")


def compare(manifest, old, new):
    validate_manifest(manifest)
    findings = []
    for label, snapshot in (("baseline", old), ("candidate", new)):
        if snapshot.get("schema") != 1 or snapshot.get("manifest_sha256") != digest(manifest):
            finding(findings, label, "error", "snapshot-policy-mismatch", "", "Schema or manifest digest does not match")
        for error in snapshot.get("errors", []):
            finding(findings, error["source"], "error", "capture-failed", label, error["message"])
    for spec in manifest["sources"]:
        name, kind = spec["id"], spec["kind"]
        a, b = old.get("sources", {}).get(name), new.get("sources", {}).get(name)
        if a is None or b is None:
            finding(findings, name, "error", "source-missing", "", "Required source snapshot is missing")
            continue
        if a.get("kind") != kind or b.get("kind") != kind:
            finding(findings, name, "error", "source-kind-mismatch", "", "Source kind does not match manifest")
            continue
        local = []
        try:
            validate_source(spec, a)
            validate_source(spec, b)
            if kind in ("schema", "openapi"):
                schema_changes(a["data"], b["data"], name, local)
            elif kind == "codex-schema":
                for filename in spec["files"]:
                    schema_changes(a["data"][filename], b["data"][filename], name, local)
            elif kind == "cli":
                for command in spec["commands"]:
                    key = command["name"]
                    af, bf = a["data"][key], b["data"][key]
                    required = set(command["required_flags"])
                    for flag in sorted(required - bf.keys()):
                        finding(local, name, "breaking", "cli-required-flag-missing", key + "/" + flag,
                                "A flag used by this product is missing")
                    for flag in sorted(af.keys() | bf.keys()):
                        if flag not in bf and flag not in required:
                            finding(local, name, "info", "cli-unused-flag-removed", key + "/" + flag, "Unselected flag removed")
                        elif flag not in af:
                            finding(local, name, "info", "cli-flag-added", key + "/" + flag, "Flag added")
                        elif flag in bf and af[flag] != bf[flag]:
                            finding(local, name, "review" if flag in required else "info", "cli-help-changed",
                                    key + "/" + flag, "Flag signature/help changed")
            elif kind == "python-types":
                ad, bd = a["data"]["declarations"], b["data"]["declarations"]
                if a["data"]["imports"] != b["data"]["imports"]:
                    finding(local, name, "review", "sdk-imports-changed", "imports", "Referenced SDK types/imports changed")
                for symbol in sorted(ad.keys() | bd.keys()):
                    if symbol not in bd:
                        finding(local, name, "breaking", "sdk-type-removed", symbol, "SDK declaration removed")
                    elif symbol not in ad:
                        finding(local, name, "info", "sdk-type-added", symbol, "SDK declaration added")
                    elif ad[symbol] != bd[symbol]:
                        finding(local, name, "review", "sdk-type-changed", symbol, "SDK declaration changed")
            elif a["data"] != b["data"]:
                finding(local, name, "review", "auth-source-changed", "", "Monitored authentication source changed")
        except (KeyError, TypeError, ValueError, RecursionError) as error:
            finding(local, name, "error", "snapshot-invalid", "", "Cannot compare snapshot: " + type(error).__name__)
        if spec.get("usage") == "advisory":
            for item in local:
                if item["level"] == "breaking":
                    item["level"] = "review"
        findings.extend(local)
        if a.get("version") != b.get("version"):
            finding(findings, name, "info", "cli-version-changed", "version", "CLI version changed: " +
                    str(a.get("version")) + " -> " + str(b.get("version")))
    findings.sort(key=lambda f: (-LEVELS[f["level"]], f["source"], f["path"], f["rule"]))
    status = max((LEVELS[item["level"]] for item in findings), default=0)
    return {"schema": 1, "scope": "static-contracts-only", "status":
            "error" if status == 3 else "hold" if status else "pass",
            "findings": findings}


def markdown(report):
    lines = ["# Upstream static contract report", "", "Status: **" + report["status"] + "**", "",
             "Static declarations only; this does not verify live API/OAuth compatibility.", "",
             "| Level | Source | Rule | Location | Detail |", "|---|---|---|---|---|"]
    def escape(value):
        return str(value).replace("|", "\\|").replace("\n", " ")
    for item in report["findings"]:
        lines.append("| " + " | ".join(escape(item[k]) for k in ("level", "source", "rule", "path", "message")) + " |")
    if not report["findings"]:
        lines.append("| info | all | unchanged | — | No monitored static changes |")
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, default=DEFAULT_MANIFEST)
    sub = parser.add_subparsers(dest="command", required=True)
    collect = sub.add_parser("capture", help="Capture official artifacts and isolated CLI declarations")
    collect.add_argument("--codex", type=Path, required=True)
    collect.add_argument("--claude", type=Path, required=True)
    collect.add_argument("--out", type=Path, required=True)
    diff = sub.add_parser("compare", help="Compare against a reviewed baseline; 0=info, 1=hold, 2=error")
    diff.add_argument("--baseline", type=Path, default=ROOT / "compat/upstream/baseline.json")
    diff.add_argument("--candidate", type=Path, required=True)
    diff.add_argument("--json", type=Path, required=True)
    diff.add_argument("--markdown", type=Path, required=True)
    validate = sub.add_parser("validate-baseline", help="Offline baseline consistency and worker CLI pin check")
    validate.add_argument("--baseline", type=Path, default=ROOT / "compat/upstream/baseline.json")
    args = parser.parse_args()
    manifest = read_json(args.manifest)
    if args.command == "validate-baseline":
        baseline = read_json(args.baseline)
        report = compare(manifest, baseline, baseline)
        dockerfile = (ROOT / "docker/sandbox/Dockerfile").read_text()
        for source, arg in (("codex-cli", "CODEX_CLI_VERSION"), ("claude-cli", "CLAUDE_CLI_VERSION")):
            match = re.search(r"^ARG " + arg + r"=([\d.]+)$", dockerfile, re.MULTILINE)
            version = baseline.get("sources", {}).get(source, {}).get("version", "")
            if not match or not re.search(r"(?<![\d.])" + re.escape(match[1]) + r"(?![\d.])", version):
                finding(report["findings"], source, "error", "worker-pin-mismatch", "version", "Baseline CLI version does not match worker Dockerfile")
                report["status"] = "error"
        print(markdown(report))
        return 0 if report["status"] == "pass" else 2
    if args.command == "capture":
        snapshot = capture(manifest, {"codex": args.codex, "claude": args.claude})
        write_json(args.out, snapshot)
        print("Captured", len(snapshot["sources"]), "sources; errors:", len(snapshot["errors"]))
        for error in snapshot["errors"]:
            print(error["source"] + ": " + error["message"], file=sys.stderr)
        return 2 if snapshot["errors"] else 0
    report = compare(manifest, read_json(args.baseline), read_json(args.candidate))
    write_json(args.json, report)
    args.markdown.parent.mkdir(parents=True, exist_ok=True)
    args.markdown.write_text(markdown(report))
    print("Static contract result:", report["status"], "(" + str(len(report["findings"])) + " findings)")
    return 2 if report["status"] == "error" else 1 if report["status"] == "hold" else 0


if __name__ == "__main__":
    sys.exit(main())
