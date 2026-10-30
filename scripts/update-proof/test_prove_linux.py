#!/usr/bin/env python3
"""Unit checks for the proof's coordinator identity policy, not app acceptance."""

import importlib.util
from pathlib import Path
import sys
import unittest


DIRECTORY = Path(__file__).resolve().parent
sys.path.insert(0, str(DIRECTORY))
SPEC = importlib.util.spec_from_file_location("update_proof_linux", DIRECTORY / "prove-linux.py")
proof = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(proof)


class CoordinatorTests(unittest.TestCase):
    def test_version_increase_requires_candidate_coordinator(self):
        initial = [{"executable": "/old/nocx-server", "sha256": "old-server"}]
        self.assertEqual(proof.permitted_coordinators(
            {"version": "0.8.0"}, {"version": "0.8.1"}, initial,
            {"coordinator": "new-server"}), {"new-server"})

    def test_equal_version_permits_existing_coordinator(self):
        initial = [{"executable": "/old/nocx-server", "sha256": "old-server"},
                   {"executable": "/old/nocx", "sha256": "old-window"}]
        self.assertEqual(proof.permitted_coordinators(
            {"version": "0.8.0"}, {"version": "0.8.0"}, initial,
            {"coordinator": "new-server"}), {"new-server", "old-server"})


if __name__ == "__main__":
    unittest.main()
