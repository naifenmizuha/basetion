#!/usr/bin/env python3
"""Evaluate Basetion batch-test JSONL results without calling a model or database."""

from __future__ import annotations

import argparse
import json
import math
import sys
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path
from typing import Any


class EvaluationInputError(ValueError):
    """Raised when the input does not follow the batch-test JSONL protocol."""


def parse_timestamp(value: Any, field: str, line_number: int) -> datetime:
    if not isinstance(value, str) or not value:
        raise EvaluationInputError(f"第 {line_number} 行的 {field} 必须是非空 RFC3339 时间")
    normalized = value[:-1] + "+00:00" if value.endswith("Z") else value
    for pattern in ("%Y-%m-%dT%H:%M:%S.%f%z", "%Y-%m-%dT%H:%M:%S%z"):
        try:
            return datetime.strptime(normalized, pattern)
        except ValueError:
            continue
    raise EvaluationInputError(f"第 {line_number} 行的 {field} 无法解析: {value!r}")


def require_string(record: dict[str, Any], field: str, line_number: int) -> str:
    value = record.get(field)
    if not isinstance(value, str) or not value:
        raise EvaluationInputError(f"第 {line_number} 行缺少非空 {field}")
    return value


def optional_name_list(record: dict[str, Any], field: str, line_number: int) -> list[str]:
    value = record.get(field, [])
    if value is None:
        return []
    if not isinstance(value, list) or any(not isinstance(item, str) or not item for item in value):
        raise EvaluationInputError(f"第 {line_number} 行的 {field} 必须是非空字符串数组")
    if len(set(value)) != len(value):
        raise EvaluationInputError(f"第 {line_number} 行的 {field} 不得重复")
    return value


def usage_from_request(request: dict[str, Any], line_number: int) -> tuple[int, int, int] | None:
    usage = request.get("token_usage")
    if usage is None:
        return None
    if not isinstance(usage, dict):
        raise EvaluationInputError(f"第 {line_number} 行的 model_requests.token_usage 必须是对象")
    values: list[int] = []
    for field in ("prompt_tokens", "completion_tokens", "total_tokens"):
        value = usage.get(field, 0)
        if isinstance(value, bool) or not isinstance(value, int) or value < 0:
            raise EvaluationInputError(f"第 {line_number} 行的 model_requests.token_usage.{field} 必须是非负整数")
        values.append(value)
    prompt, completion, total = values
    if total == 0 and (prompt != 0 or completion != 0):
        total = prompt + completion
    if total == 0:
        return None
    return prompt, completion, total


def token_usage(model_requests: Any, line_number: int) -> tuple[dict[str, int], int, int]:
    if model_requests is None:
        model_requests = []
    if not isinstance(model_requests, list):
        raise EvaluationInputError(f"第 {line_number} 行的 model_requests 必须是数组")
    total = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
    missing = 0
    for request in model_requests:
        if not isinstance(request, dict):
            raise EvaluationInputError(f"第 {line_number} 行的 model_requests 包含非对象值")
        candidate = usage_from_request(request, line_number)
        if candidate is None:
            missing += 1
            continue
        prompt, completion, tokens = candidate
        total["prompt_tokens"] += prompt
        total["completion_tokens"] += completion
        total["total_tokens"] += tokens
    return total, len(model_requests), missing


def observed_path(events: Any, line_number: int) -> tuple[list[str], list[str], list[str]]:
    if events is None:
        events = []
    if not isinstance(events, list):
        raise EvaluationInputError(f"第 {line_number} 行的 events 必须是数组")
    tools: list[str] = []
    skills: list[str] = []
    warnings: list[str] = []
    for event in events:
        if not isinstance(event, dict) or event.get("type") != "message":
            continue
        message = event.get("message")
        if not isinstance(message, dict):
            continue
        blocks = message.get("content_blocks", [])
        if not isinstance(blocks, list):
            continue
        for block in blocks:
            if not isinstance(block, dict):
                continue
            call = block.get("function_tool_call")
            if not isinstance(call, dict):
                continue
            name = call.get("name")
            if not isinstance(name, str) or not name:
                warnings.append("unparseable_tool_call")
                continue
            if name != "skill":
                tools.append(name)
                continue
            arguments = call.get("arguments")
            try:
                decoded = json.loads(arguments) if isinstance(arguments, str) else None
            except json.JSONDecodeError:
                decoded = None
            skill = decoded.get("skill") if isinstance(decoded, dict) else None
            if not isinstance(skill, str) or not skill:
                warnings.append("unparseable_skill_call")
                continue
            skills.append(skill)
    return tools, skills, warnings


def percentile(values: list[int], percent: float) -> int | None:
    if not values:
        return None
    ordered = sorted(values)
    return ordered[math.ceil(percent * len(ordered)) - 1]


def metric_summary(values: list[int]) -> dict[str, int | None]:
    return {
        "count": len(values),
        "total": sum(values),
        "average": round(sum(values) / len(values)) if values else None,
        "p50": percentile(values, 0.50),
        "p95": percentile(values, 0.95),
        "max": max(values) if values else None,
    }


def evaluate_record(record: dict[str, Any], line_number: int) -> dict[str, Any]:
    if record.get("schema_version") != 2:
        raise EvaluationInputError(f"第 {line_number} 行的 schema_version 必须为 2")
    run_id = require_string(record, "run_id", line_number)
    run_name = require_string(record, "run_name", line_number)
    session_id = require_string(record, "session_id", line_number)
    status = require_string(record, "status", line_number)
    session_index = record.get("session_index")
    if isinstance(session_index, bool) or not isinstance(session_index, int) or session_index <= 0:
        raise EvaluationInputError(f"第 {line_number} 行的 session_index 必须是正整数")
    turn_index = record.get("turn_index")
    if isinstance(turn_index, bool) or not isinstance(turn_index, int) or turn_index <= 0:
        raise EvaluationInputError(f"第 {line_number} 行的 turn_index 必须是正整数")
    started_at = parse_timestamp(record.get("started_at"), "started_at", line_number)
    finished_at = parse_timestamp(record.get("finished_at"), "finished_at", line_number)
    duration_ms = round((finished_at - started_at).total_seconds() * 1000)
    if duration_ms < 0:
        raise EvaluationInputError(f"第 {line_number} 行的 finished_at 早于 started_at")

    expect_tools = optional_name_list(record, "expect_tools", line_number)
    expect_skills = optional_name_list(record, "expect_skills", line_number)
    forbid_tools = optional_name_list(record, "forbid_tools", line_number)
    usage, request_count, missing_usage = token_usage(record.get("model_requests"), line_number)
    tools, skills, warnings = observed_path(record.get("events"), line_number)
    violations: list[str] = []
    if status != "completed":
        violations.append("turn_failed")
    observed_tools = set(tools)
    observed_skills = set(skills)
    for name in expect_tools:
        if name not in observed_tools:
            violations.append(f"missing_expected_tool:{name}")
    for name in expect_skills:
        if name not in observed_skills:
            violations.append(f"missing_expected_skill:{name}")
    for name in forbid_tools:
        if name in observed_tools:
            violations.append(f"forbidden_tool_called:{name}")
    if expect_tools:
        warnings.extend(f"unexpected_tool:{name}" for name in sorted(observed_tools - set(expect_tools) - set(forbid_tools)))
    if expect_skills:
        warnings.extend(f"unexpected_skill:{name}" for name in sorted(observed_skills - set(expect_skills)))
    warnings.extend(f"repeated_tool:{name}" for name, count in Counter(tools).items() if count > 1)
    warnings.extend(f"repeated_skill:{name}" for name, count in Counter(skills).items() if count > 1)
    if missing_usage:
        warnings.append(f"missing_token_usage:{missing_usage}")

    return {
        "run_id": run_id,
        "run_name": run_name,
        "session_id": session_id,
        "session_index": session_index,
        "turn_index": turn_index,
        "status": status,
        "started_at": record["started_at"],
        "finished_at": record["finished_at"],
        "duration_ms": duration_ms,
        "model_requests": request_count,
        "requests_without_token_usage": missing_usage,
        "token_usage": usage,
        "observed_tools": tools,
        "observed_skills": skills,
        "expect_tools": expect_tools,
        "expect_skills": expect_skills,
        "forbid_tools": forbid_tools,
        "warnings": sorted(set(warnings)),
        "violations": violations,
        "verdict": "failed" if violations else "passed",
    }


def evaluate_file(input_path: Path) -> dict[str, Any]:
    turns: list[dict[str, Any]] = []
    try:
        lines = input_path.read_text(encoding="utf-8").splitlines()
    except OSError as error:
        raise EvaluationInputError(f"读取 {input_path} 失败: {error}") from error
    for line_number, line in enumerate(lines, start=1):
        if not line.strip():
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError as error:
            raise EvaluationInputError(f"第 {line_number} 行不是有效 JSON: {error.msg}") from error
        if not isinstance(record, dict):
            raise EvaluationInputError(f"第 {line_number} 行必须是 JSON 对象")
        turns.append(evaluate_record(record, line_number))
    if not turns:
        raise EvaluationInputError("结果 JSONL 不包含任何记录")

    grouped: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
    for turn in turns:
        grouped[(turn["run_id"], turn["run_name"])].append(turn)
    runs = []
    for (run_id, run_name), run_turns in sorted(grouped.items()):
        runs.append(summarize_run(run_id, run_name, run_turns))
    return {
        "schema_version": 2,
        "input": str(input_path),
        "summary": summarize_turns(turns),
        "runs": runs,
        "turns": turns,
    }


def summarize_turns(turns: list[dict[str, Any]]) -> dict[str, Any]:
    durations = [turn["duration_ms"] for turn in turns]
    token_fields = ("prompt_tokens", "completion_tokens", "total_tokens")
    return {
        "total_turns": len(turns),
        "completed_turns": sum(turn["status"] == "completed" for turn in turns),
        "failed_turns": sum(turn["status"] != "completed" for turn in turns),
        "violating_turns": sum(bool(turn["violations"]) for turn in turns),
        "warning_count": sum(len(turn["warnings"]) for turn in turns),
        "duration_ms": metric_summary(durations),
        "wall_clock_ms": wall_clock_ms(turns),
        "token_usage": {
            **{field: sum(turn["token_usage"][field] for turn in turns) for field in token_fields},
            "model_requests": sum(turn["model_requests"] for turn in turns),
            "requests_without_token_usage": sum(turn["requests_without_token_usage"] for turn in turns),
        },
    }


def summarize_run(run_id: str, run_name: str, turns: list[dict[str, Any]]) -> dict[str, Any]:
    summary = summarize_turns(turns)
    return {"run_id": run_id, "run_name": run_name, **summary}


def wall_clock_ms(turns: list[dict[str, Any]]) -> int:
    starts = [parse_timestamp(turn["started_at"], "started_at", 0) for turn in turns]
    finishes = [parse_timestamp(turn["finished_at"], "finished_at", 0) for turn in turns]
    return round((max(finishes) - min(starts)).total_seconds() * 1000)


def default_output_path(input_path: Path) -> Path:
    suffix = ".evaluation.json"
    if input_path.suffix:
        return input_path.with_suffix(suffix)
    return input_path.with_name(input_path.name + suffix)


def default_report_path(input_path: Path) -> Path:
    suffix = ".report.md"
    if input_path.suffix:
        return input_path.with_suffix(suffix)
    return input_path.with_name(input_path.name + suffix)


def write_report(path: Path, report: dict[str, Any]) -> None:
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    except OSError as error:
        raise EvaluationInputError(f"写入评测报告 {path} 失败: {error}") from error


def markdown_code(value: Any, language: str = "") -> list[str]:
    text = value if isinstance(value, str) else json.dumps(value, ensure_ascii=False, indent=2)
    return [f"````{language}", text, "````"]


def reasoning_text(value: Any) -> str:
    if not isinstance(value, dict):
        return ""
    extension = value.get("openai_extension")
    if not isinstance(extension, dict) or not isinstance(extension.get("content"), list):
        return ""
    return "".join(item.get("text", "") for item in extension["content"] if isinstance(item, dict))


def tool_result_text(value: Any) -> str:
    if not isinstance(value, dict) or not isinstance(value.get("content"), list):
        return json.dumps(value, ensure_ascii=False, indent=2)
    parts: list[str] = []
    for item in value["content"]:
        text = item.get("text") if isinstance(item, dict) else None
        if isinstance(text, dict) and isinstance(text.get("text"), str):
            parts.append(text["text"])
        else:
            parts.append(json.dumps(item, ensure_ascii=False, indent=2))
    return "\n".join(parts)


def render_event_markdown(events: Any) -> list[str]:
    lines: list[str] = []
    if not isinstance(events, list):
        return lines
    for event in events:
        if not isinstance(event, dict):
            continue
        if event.get("type") == "action":
            lines.extend(["#### 动作", *markdown_code(event.get("action"), "json"), ""])
            continue
        message = event.get("message")
        blocks = message.get("content_blocks", []) if isinstance(message, dict) else []
        if not isinstance(blocks, list):
            continue
        for block in blocks:
            if not isinstance(block, dict):
                continue
            if isinstance(block.get("reasoning"), dict):
                text = reasoning_text(block["reasoning"])
                if text:
                    lines.extend(["#### 思考", text, ""])
            elif isinstance(block.get("assistant_gen_text"), dict):
                text = block["assistant_gen_text"].get("text")
                if isinstance(text, str) and text:
                    lines.extend(["#### 回复", text, ""])
            elif isinstance(block.get("function_tool_call"), dict):
                call = block["function_tool_call"]
                lines.append(f"#### 工具调用 `{call.get('name', 'unknown')}`")
                lines.append(f"Call ID: `{call.get('call_id', '')}`")
                arguments = call.get("arguments", "")
                try:
                    arguments = json.loads(arguments) if isinstance(arguments, str) else arguments
                except json.JSONDecodeError:
                    pass
                lines.extend(["", *markdown_code(arguments, "json"), ""])
            elif isinstance(block.get("function_tool_result"), dict):
                result = block["function_tool_result"]
                lines.append(f"#### 工具结果 `{result.get('name', 'unknown')}`")
                lines.append(f"Call ID: `{result.get('call_id', '')}`")
                lines.extend(["", *markdown_code(tool_result_text(result)), ""])
    return lines


def write_markdown_report(path: Path, input_path: Path, report: dict[str, Any]) -> None:
    records: dict[tuple[str, str, int], dict[str, Any]] = {}
    for line in input_path.read_text(encoding="utf-8").splitlines():
        if line.strip():
            record = json.loads(line)
            records[(record["run_id"], record["run_name"], record["turn_index"])] = record
    summary = report["summary"]
    usage = summary["token_usage"]
    lines = [
        "# Basetion 测试报告",
        "",
        f"- 输入：`{input_path}`",
        f"- 轮次：{summary['total_turns']}（完成 {summary['completed_turns']}，失败 {summary['failed_turns']}）",
        f"- 判定：违规 {summary['violating_turns']}，警告 {summary['warning_count']}",
        f"- 耗时：wall {summary['wall_clock_ms']} ms，累计 {summary['duration_ms']['total']} ms",
        f"- Token：prompt {usage['prompt_tokens']}，completion {usage['completion_tokens']}，total {usage['total_tokens']}，请求 {usage['model_requests']}",
        "",
    ]
    turns = sorted(report["turns"], key=lambda turn: (turn["session_index"], turn["turn_index"]))
    for turn in turns:
        source = records[(turn["run_id"], turn["run_name"], turn["turn_index"])]
        lines.extend([
            f"## {turn['run_name']} · 第 {turn['turn_index']} 轮",
            "",
            f"- 状态：`{turn['status']}`；判定：`{turn['verdict']}`；耗时：{turn['duration_ms']} ms",
            f"- Token：prompt {turn['token_usage']['prompt_tokens']}，completion {turn['token_usage']['completion_tokens']}，total {turn['token_usage']['total_tokens']}，请求 {turn['model_requests']}",
            f"- 预期工具：{', '.join(turn['expect_tools']) or '无'}；预期 Skill：{', '.join(turn['expect_skills']) or '无'}；禁用工具：{', '.join(turn['forbid_tools']) or '无'}",
            f"- 实际工具：{', '.join(turn['observed_tools']) or '无'}；实际 Skill：{', '.join(turn['observed_skills']) or '无'}",
            f"- 警告：{', '.join(turn['warnings']) or '无'}；违规：{', '.join(turn['violations']) or '无'}",
            "",
            "### Prompt",
            "",
            *markdown_code(source.get("prompt", "")),
            "",
            "### 调用轨迹",
            "",
            *render_event_markdown(source.get("events")),
        ])
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("\n".join(lines).rstrip() + "\n", encoding="utf-8")
    except (OSError, KeyError, json.JSONDecodeError) as error:
        raise EvaluationInputError(f"写入 Markdown 报告 {path} 失败: {error}") from error


def print_summary(report: dict[str, Any], output_path: Path, markdown_path: Path) -> None:
    summary = report["summary"]
    duration = summary["duration_ms"]
    usage = summary["token_usage"]
    print(f"评测输入: {report['input']}")
    print(f"轮次: {summary['total_turns']}，完成: {summary['completed_turns']}，违规: {summary['violating_turns']}，警告: {summary['warning_count']}")
    print(f"耗时(ms): sum={duration['total']} wall={summary['wall_clock_ms']} avg={duration['average']} p50={duration['p50']} p95={duration['p95']} max={duration['max']}")
    print(f"Token: prompt={usage['prompt_tokens']} completion={usage['completion_tokens']} total={usage['total_tokens']} requests={usage['model_requests']} missing_usage={usage['requests_without_token_usage']}")
    for turn in report["turns"]:
        if turn["violations"]:
            print(f"违规: run={turn['run_name']} turn={turn['turn_index']} {', '.join(turn['violations'])}")
    print(f"报告: {output_path}")
    print(f"可读报告: {markdown_path}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results", type=Path, help="批量测试产生的 JSONL 文件")
    parser.add_argument("--output", type=Path, help="评测 JSON 输出路径；默认与输入同名并使用 .evaluation.json 后缀")
    parser.add_argument("--report-output", type=Path, help="Markdown 报告路径；默认与输入同名并使用 .report.md 后缀")
    args = parser.parse_args()
    try:
        report = evaluate_file(args.results)
        output_path = args.output or default_output_path(args.results)
        markdown_path = args.report_output or default_report_path(args.results)
        write_report(output_path, report)
        write_markdown_report(markdown_path, args.results, report)
    except EvaluationInputError as error:
        print(f"评测输入错误: {error}", file=sys.stderr)
        return 2
    print_summary(report, output_path, markdown_path)
    return 1 if report["summary"]["violating_turns"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
