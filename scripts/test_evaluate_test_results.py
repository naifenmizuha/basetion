#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from evaluate_test_results import EvaluationInputError, evaluate_file, write_markdown_report


def record(**overrides: object) -> dict[str, object]:
    value: dict[str, object] = {
        "schema_version": 2,
        "run_id": "run-1",
        "run_name": "team-roster",
        "session_id": "session-1",
        "session_index": 1,
        "turn_index": 1,
        "started_at": "2026-08-20T03:00:00Z",
        "finished_at": "2026-08-20T03:00:02.250Z",
        "status": "completed",
        "expect_tools": ["team_query"],
        "expect_skills": ["manage-team"],
        "forbid_tools": ["team_modify"],
        "events": [
            {
                "type": "message",
                "message": {
                    "content_blocks": [
                        {"function_tool_call": {"name": "skill", "arguments": '{"skill":"manage-team"}'}},
                        {"function_tool_call": {"name": "team_query", "arguments": "{}"}},
                    ]
                },
            }
        ],
        "model_requests": [
            {
                "index": 1,
                "token_usage": {"prompt_tokens": 7, "completion_tokens": 4, "total_tokens": 11},
            },
            {"index": 2},
        ],
    }
    value.update(overrides)
    return value


class EvaluateTestResultsTest(unittest.TestCase):
    def write_records(self, *records: dict[str, object]) -> Path:
        temporary = tempfile.NamedTemporaryFile(mode="w", suffix=".jsonl", delete=False, encoding="utf-8")
        self.addCleanup(lambda: Path(temporary.name).unlink(missing_ok=True))
        with temporary:
            for value in records:
                temporary.write(json.dumps(value) + "\n")
        return Path(temporary.name)

    def test_evaluates_token_usage_once_per_model_request(self) -> None:
        report = evaluate_file(self.write_records(record()))
        summary = report["summary"]
        self.assertEqual(1, summary["total_turns"])
        self.assertEqual(2250, summary["duration_ms"]["total"])
        self.assertEqual(2250, summary["wall_clock_ms"])
        self.assertEqual(7, summary["token_usage"]["prompt_tokens"])
        self.assertEqual(4, summary["token_usage"]["completion_tokens"])
        self.assertEqual(11, summary["token_usage"]["total_tokens"])
        self.assertEqual(2, summary["token_usage"]["model_requests"])
        self.assertEqual(1, summary["token_usage"]["requests_without_token_usage"])
        turn = report["turns"][0]
        self.assertEqual([], turn["violations"])
        self.assertEqual(["team_query"], turn["observed_tools"])
        self.assertEqual(["manage-team"], turn["observed_skills"])

    def test_reports_failed_missing_and_forbidden_path_violations(self) -> None:
        value = record(
            status="failed",
            expect_tools=["team_fetch"],
            expect_skills=["project-knowledge"],
            forbid_tools=["team_query"],
        )
        report = evaluate_file(self.write_records(value))
        violations = report["turns"][0]["violations"]
        self.assertEqual(
            [
                "turn_failed",
                "missing_expected_tool:team_fetch",
                "missing_expected_skill:project-knowledge",
                "forbidden_tool_called:team_query",
            ],
            violations,
        )
        self.assertEqual(1, report["summary"]["violating_turns"])

    def test_rejects_invalid_jsonl(self) -> None:
        temporary = tempfile.NamedTemporaryFile(mode="w", suffix=".jsonl", delete=False, encoding="utf-8")
        self.addCleanup(lambda: Path(temporary.name).unlink(missing_ok=True))
        with temporary:
            temporary.write("not-json\n")
        with self.assertRaises(EvaluationInputError):
            evaluate_file(Path(temporary.name))

    def test_rejects_legacy_schema(self) -> None:
        with self.assertRaisesRegex(EvaluationInputError, "schema_version"):
            evaluate_file(self.write_records(record(schema_version=1)))

    def test_writes_human_readable_markdown_trace(self) -> None:
        value = record(
            prompt="列出球队",
            events=[
                {"type": "message", "message": {"content_blocks": [
                    {"reasoning": {"openai_extension": {"content": [{"text": "先查询"}]}}},
                    {"function_tool_call": {"name": "team_query", "call_id": "call-1", "arguments": '{"mode":"query"}'}},
                    {"function_tool_result": {"name": "team_query", "call_id": "call-1", "content": [{"text": {"text": '{"teams":2}'}}]}},
                    {"assistant_gen_text": {"text": "共有两支球队。"}},
                ]}},
            ],
        )
        input_path = self.write_records(value)
        output_path = input_path.with_suffix(".report.md")
        self.addCleanup(lambda: output_path.unlink(missing_ok=True))
        write_markdown_report(output_path, input_path, evaluate_file(input_path))
        markdown = output_path.read_text(encoding="utf-8")
        for expected in ("列出球队", "先查询", "team_query", "call-1", "teams", "共有两支球队。"):
            self.assertIn(expected, markdown)


if __name__ == "__main__":
    unittest.main()
