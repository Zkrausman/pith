"""Exercise the installed adapter contract against a compiled Pith binary."""

from __future__ import annotations

import json
import os
import time
import unittest
from pathlib import Path

import pre_tool_use_pith as adapter


PITH_BINARY = os.environ.get("PITH_GO_SUCCESS_TEST_BINARY", "")
GO = "/home/zack/Documents/Codex/2026-10-10/task/pith-go1261-setup-xGlpXdZS/go/bin/go"
COMMAND = f"{GO} test -v ./pkg/telemetry"
RAW = (
    b"=== RUN   TestParent\n"
    b"=== RUN   TestParent/ErrorWord\n"
    b"    --- PASS: TestParent/ErrorWord (0.00s)\n"
    b"=== RUN   TestParent/FailWord\n"
    b"    --- PASS: TestParent/FailWord (0.01s)\n"
    b"--- PASS: TestParent (0.02s)\n"
    b"=== RUN   TestStandaloneError\n"
    b"--- PASS: TestStandaloneError (0.03s)\n"
    b"PASS\n"
    b"ok  \tpith/pkg/telemetry\t1.206s\n"
)
SELECTED = b"PASS\nok  \tpith/pkg/telemetry\t1.206s"


class CompiledTransformContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        if not PITH_BINARY:
            raise RuntimeError("PITH_GO_SUCCESS_TEST_BINARY must name the compiled Pith binary")
        binary = Path(PITH_BINARY)
        if not binary.is_absolute() or not binary.is_file() or not os.access(binary, os.X_OK):
            raise RuntimeError(f"compiled Pith binary is missing or not executable: {binary}")

    def transform(self, raw: bytes) -> bytes | None:
        response = adapter._run_transform(
            PITH_BINARY, COMMAND, raw, time.monotonic() + adapter.TRANSFORM_SECONDS,
        )
        if response is None:
            return None
        selected = adapter._selected_output(raw, response)
        return selected

    def test_compiled_transform_selects_nested_success_summary(self) -> None:
        response = adapter._run_transform(
            PITH_BINARY, COMMAND, RAW, time.monotonic() + adapter.TRANSFORM_SECONDS,
        )
        self.assertIsNotNone(response, "compiled Pith transform did not return a valid response")
        assert response is not None
        payload = json.loads(response)
        self.assertEqual(payload.get("parser"), "tests")
        self.assertIs(payload.get("passthrough"), False)
        self.assertEqual(adapter._selected_output(RAW, response), SELECTED)

    def test_diagnostic_output_falls_back_before_transform(self) -> None:
        diagnostic = RAW.replace(b"--- PASS:", b"warning: fixture diagnostic\n--- PASS:", 1)
        indented_diagnostic = RAW.replace(b"=== RUN   TestParent\n", b"=== RUN   TestParent\n    warning: fixture diagnostic\n", 1)
        self.assertIsNone(self.transform(diagnostic))
        self.assertIsNone(self.transform(indented_diagnostic))

    def test_duplicate_or_malformed_output_falls_back(self) -> None:
        duplicate = RAW.replace(
            b"    --- PASS: TestParent/ErrorWord (0.00s)\n",
            b"    --- PASS: TestParent/ErrorWord (0.00s)\n"
            b"    --- PASS: TestParent/ErrorWord (0.00s)\n",
            1,
        )
        malformed = RAW.replace(b"0.01s", b"NaNs", 1)
        malformed_prefix = RAW.replace(b"ok  \tpith/", b"xok  \tpith/", 1)
        self.assertIsNone(self.transform(duplicate))
        self.assertIsNone(self.transform(malformed))
        self.assertIsNone(self.transform(malformed_prefix))


if __name__ == "__main__":
    unittest.main(verbosity=2)
