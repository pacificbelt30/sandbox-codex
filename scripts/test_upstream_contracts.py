"""Contract drift must hold upgrades without trusting executable upstream code."""
import copy
import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location("upstream_contracts", Path(__file__).with_name("upstream-contracts.py"))
CHECK = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECK)


def bundle(schema, refs=None, root="#"):
    return {"roots": {root: schema}, "refs": refs or {}}


def schema_findings(old, new):
    findings = []
    CHECK.schema_changes(old, new, "fixture", findings)
    return findings


class SchemaContractTests(unittest.TestCase):
    def levels(self, old, new):
        return [f["level"] for f in schema_findings(old, new)]

    def test_required_fields_are_directional(self):
        before = {"type": "object", "required": ["model"], "properties": {"model": {"type": "string"}}}
        after = {**before, "required": ["model", "tenant"]}
        self.assertIn("breaking", self.levels(bundle(before), bundle(after)))
        old = {"responses": {"200": before}}
        new = {"responses": {"200": after}}
        self.assertEqual(self.levels(bundle(old), bundle(new)), ["info"])
        self.assertIn("breaking", self.levels(bundle(new), bundle(old)))

    def test_first_required_constraint_is_not_assumed_compatible(self):
        self.assertIn("breaking", self.levels(bundle({"type": "object"}),
                                            bundle({"type": "object", "required": ["token"]})))

    def test_required_body_boolean_is_directional(self):
        old = bundle({"requestBody": {"required": False}})
        new = bundle({"requestBody": {"required": True}})
        self.assertEqual(self.levels(old, new), ["breaking"])

    def test_request_enum_narrows_and_response_enum_expands(self):
        before, after = {"enum": ["a", "b"]}, {"enum": ["a"]}
        self.assertEqual(self.levels(bundle(before), bundle(after)), ["breaking"])
        self.assertEqual(self.levels(bundle(after), bundle(before)), ["info"])
        self.assertEqual(self.levels(bundle({"responses": before}), bundle({"responses": after})), ["info"])
        self.assertEqual(self.levels(bundle({"responses": after}), bundle({"responses": before})), ["breaking"])

    def test_optional_property_and_new_validation_constraint(self):
        a = {"type": "object", "properties": {}}
        b = {"type": "object", "properties": {"extra": {"type": "string"}}}
        self.assertEqual(self.levels(bundle(a), bundle(b)), ["info"])
        self.assertEqual(self.levels(bundle(b), bundle(a)), ["breaking"])
        self.assertEqual(self.levels(bundle({"type": "string"}), bundle({"type": "string", "pattern": "^a"})), ["review"])

    def test_referenced_type_change_is_found_when_root_is_identical(self):
        root = {"$ref": "#/definitions/Request"}
        old = bundle(root, {"#/definitions/Request": {"type": "string"}})
        new = bundle(root, {"#/definitions/Request": {"type": "integer"}})
        self.assertEqual(self.levels(old, new), ["breaking"])

    def test_nested_reference_in_unchanged_composition_is_found(self):
        ref = "#/definitions/Request"
        root = {"anyOf": [{"$ref": ref}, {"type": "null"}]}
        old = bundle(root, {ref: {"type": "string"}})
        new = bundle(root, {ref: {"type": "integer"}})
        self.assertIn("breaking", self.levels(old, new))

    def test_cycles_terminate_and_still_detect_nested_change(self):
        ref = "#/definitions/Node"
        node = {"type": "object", "properties": {"next": {"$ref": ref}, "model": {"type": "string"}}}
        changed = copy.deepcopy(node)
        changed["properties"]["model"]["type"] = "integer"
        self.assertIn("breaking", self.levels(bundle({"$ref": ref}, {ref: node}),
                                            bundle({"$ref": ref}, {ref: changed})))

    def test_ref_siblings_and_unknown_keywords_are_reviewed(self):
        ref = "#/definitions/T"
        old = bundle({"$ref": ref}, {ref: {"type": "string"}})
        new = bundle({"$ref": ref, "minLength": 2}, {ref: {"type": "string"}})
        self.assertIn("review", self.levels(old, new))
        self.assertEqual(self.levels(bundle({"x-new-validation": 1}), bundle({"x-new-validation": 2})), ["review"])

    def test_shared_reference_does_not_hide_changed_sibling_constraints(self):
        old_ref, new_ref = "#/definitions/Old", "#/definitions/Renamed"
        old_root = {"properties": {"first": {"$ref": old_ref}, "second": {"$ref": old_ref}}}
        new_root = {"properties": {"first": {"$ref": new_ref}, "second": {"$ref": new_ref, "minLength": 3}}}
        before = bundle(old_root, {old_ref: {"type": "string"}})
        after = bundle(new_root, {new_ref: {"type": "string"}})
        self.assertIn("review", self.levels(before, after))

    def test_security_changes_never_automatically_pass(self):
        old, new = bundle({"security": [{"apiKey": []}]}), bundle({"security": []})
        self.assertIn("review", self.levels(old, new))
        self.assertIn("review", self.levels(bundle({"servers": [{"url": "https://a.example"}]}),
                                           bundle({"servers": [{"url": "https://b.example"}]})))

    def test_unrelated_reference_does_not_change_identical_security(self):
        old = bundle({"security": [{"apiKey": []}]}, {"#/definitions/X": {"type": "string"}})
        new = bundle({"security": [{"apiKey": []}]}, {"#/definitions/X": {"type": "number"}})
        self.assertEqual(self.levels(old, new), [])

    def test_missing_and_permanently_unavailable_operations(self):
        self.assertEqual(self.levels(bundle({"requestBody": {}}), bundle(None)), ["breaking"])
        self.assertEqual(self.levels(bundle(None), bundle(None)), ["review"])

    def test_documentation_changes_are_ignored_without_losing_named_fields(self):
        a = {"description": "one", "properties": {"title": {"type": "string", "description": "old"}}}
        b = {"description": "two", "properties": {"title": {"type": "string", "description": "new"}}}
        self.assertEqual(CHECK.clean(a), CHECK.clean(b))
        self.assertIn("title", CHECK.clean(a)["properties"])
        self.assertEqual(CHECK.clean(a)["properties"]["title"], {"type": "string"})

    def test_reference_closure_and_external_ref_rejection(self):
        doc = {"definitions": {"A": {"$ref": "#/definitions/B"}, "B": {"$ref": "#/definitions/A"}, "Unused": {}}}
        result = CHECK.schema_bundle(doc, {"request": {"$ref": "#/definitions/A"}})
        self.assertEqual(set(result["refs"]), {"#/definitions/A", "#/definitions/B"})
        with self.assertRaisesRegex(ValueError, "external"):
            CHECK.schema_bundle({}, {"request": {"$ref": "https://example.com/schema"}})


class CaptureAndPolicyTests(unittest.TestCase):
    def setUp(self):
        self.spec = {"id": "fixture", "kind": "schema", "url": "https://example.com/schema", "roots": ["#"]}
        self.manifest = {"schema": 1, "sources": [self.spec]}

    def snapshot(self, data):
        return {"schema": 1, "manifest_sha256": CHECK.digest(self.manifest), "errors": [],
                "sources": {"fixture": {"kind": self.spec["kind"], "data": data, "version": "fixture 1"}}}

    def test_unchanged_report_and_manifest_mismatch(self):
        old = self.snapshot(bundle({"type": "string"}))
        self.assertEqual(CHECK.compare(self.manifest, old, old)["status"], "pass")
        invalid = copy.deepcopy(old)
        invalid["manifest_sha256"] = "wrong"
        self.assertEqual(CHECK.compare(self.manifest, old, invalid)["status"], "error")

    def test_incomplete_capture_does_not_pass(self):
        with patch.object(CHECK, "fetch", side_effect=OSError("unavailable")):
            result = CHECK.capture(self.manifest, {})
        self.assertEqual(len(result["errors"]), 1)
        self.assertEqual(CHECK.compare(self.manifest, result, result)["status"], "error")

    def test_corrupt_ref_does_not_pass(self):
        old = self.snapshot(bundle({"$ref": "#/missing"}))
        new = self.snapshot(bundle({"type": "string"}))
        self.assertEqual(CHECK.compare(self.manifest, old, new)["status"], "error")
        self.assertEqual(CHECK.compare(self.manifest, old, old)["status"], "error")

    def test_missing_selected_root_does_not_pass_even_in_identical_snapshots(self):
        invalid = self.snapshot({"roots": {"unselected": {}}, "refs": {}})
        self.assertEqual(CHECK.compare(self.manifest, invalid, invalid)["status"], "error")

    def test_advisory_schema_changes_hold_without_claiming_runtime_breakage(self):
        self.spec["usage"] = "advisory"
        old, new = self.snapshot(bundle({"type": "string"})), self.snapshot(bundle({"type": "number"}))
        report = CHECK.compare(self.manifest, old, new)
        self.assertEqual(report["status"], "hold")
        self.assertEqual(report["findings"][0]["level"], "review")

    def test_sdk_parse_does_not_execute_code_or_keep_class_docstrings(self):
        before = 'raise RuntimeError("must not run")\nclass Message:\n """old docs"""\n model: str\n'
        after = before.replace("old docs", "new docs")
        self.assertEqual(CHECK.python_types(before), CHECK.python_types(after))
        self.assertNotEqual(CHECK.python_types(before), CHECK.python_types(after.replace("model: str", "model: int")))
        with self.assertRaisesRegex(ValueError, "no SDK"):
            CHECK.python_types('print("not types")')

    def test_sdk_type_change_is_a_hold(self):
        self.spec["kind"] = "python-types"
        before = CHECK.python_types("class Message:\n model: str\n")
        after = CHECK.python_types("class Message:\n model: int\n")
        report = CHECK.compare(self.manifest, self.snapshot(before), self.snapshot(after))
        self.assertEqual(report["status"], "hold")
        self.assertEqual(report["findings"][0]["rule"], "sdk-type-changed")

    def test_cli_missing_required_flag_is_caught_even_if_baseline_also_missing(self):
        self.spec.update(kind="cli", commands=[{"name": "root", "argv": [], "required_flags": ["--config"]}])
        old = self.snapshot({"root": {"--help": "--help"}})
        report = CHECK.compare(self.manifest, old, old)
        self.assertEqual(report["status"], "hold")
        self.assertEqual(report["findings"][0]["rule"], "cli-required-flag-missing")

    def test_cli_required_flag_signature_change(self):
        self.spec.update(kind="cli", commands=[{"name": "root", "argv": [], "required_flags": ["--config"]}])
        old = self.snapshot({"root": {"--config": "--config <FILE>"}})
        new = self.snapshot({"root": {"--config": "--config <JSON>"}})
        self.assertEqual(CHECK.compare(self.manifest, old, new)["status"], "hold")

    def test_help_wrapping_and_aliases(self):
        text = "Options:\n  -p, --print <text>  Print output\n      wrapped help\n  --output-format <format>  text, json\n\n"
        flags = CHECK.help_flags(text)
        self.assertEqual(set(flags), {"--print", "--output-format"})
        self.assertIn("wrapped help", flags["--print"])
        with self.assertRaisesRegex(ValueError, "no CLI flags"):
            CHECK.help_flags("a changed format")

    def test_cli_environment_does_not_inherit_credentials_or_home(self):
        class Result:
            returncode = 0
            stdout = b"fixture version"
        with tempfile.TemporaryDirectory() as temp, patch.dict(os.environ, {"OPENAI_API_KEY": "test-secret", "CLAUDE_CODE_OAUTH_TOKEN": "test-secret", "SSH_AUTH_SOCK": "/secret"}):
            with patch.object(CHECK.subprocess, "run", return_value=Result()) as run:
                CHECK.cli_run("/fixture/codex", ["--version"], Path(temp))
            env = run.call_args.kwargs["env"]
            for key in ("OPENAI_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "SSH_AUTH_SOCK"):
                self.assertNotIn(key, env)
            self.assertEqual(env["HOME"], temp)
            self.assertEqual(run.call_args.kwargs["cwd"], Path(temp))

    def test_fetch_rejects_credentials_and_insecure_urls(self):
        for url in ("http://example.com/schema", "https://user:pass@example.com/schema", "file:///etc/passwd"):
            with self.subTest(url=url), self.assertRaises(ValueError):
                CHECK.fetch(url)

    def test_github_refs_resolve_once_per_repository_and_ref(self):
        revisions = {}
        sha = "a" * 40
        with patch.object(CHECK, "fetch", return_value=('{"sha":"' + sha + '"}').encode()) as fetch:
            first = CHECK.resolve_source("https://raw.githubusercontent.com/example/sdk/main/types.py", revisions)
            second = CHECK.resolve_source("https://raw.githubusercontent.com/example/sdk/main/events.py", revisions)
        self.assertEqual(fetch.call_count, 1)
        self.assertIn("/" + sha + "/", first)
        self.assertIn("/" + sha + "/", second)

    def test_auth_release_notes_filter_unrelated_changes(self):
        spec = {"kind": "text", "url": "https://example.com/changelog", "match_lines": "(?i)oauth"}
        with tempfile.TemporaryDirectory() as temp:
            def collect(text):
                with patch.object(CHECK, "fetch", return_value=text.encode()):
                    return CHECK.collect_source(spec, {}, Path(temp), {})["data"]
            before = collect("UI change\nOAuth fixed\n")
            self.assertEqual(before, collect("Another UI change\nOAuth fixed\n"))
            self.assertNotEqual(before, collect("UI change\nOAuth changed\n"))
            with self.assertRaisesRegex(ValueError, "no monitored"):
                collect("no relevant release notes")

    def test_report_escapes_table_delimiters(self):
        report = {"status": "hold", "findings": [{"level": "review", "source": "x", "rule": "x", "path": "a|b", "message": "a\nb"}]}
        self.assertIn("a\\|b", CHECK.markdown(report))
        self.assertIn("a b", CHECK.markdown(report))


if __name__ == "__main__":
    unittest.main()
