"""Test the package that will be archived and uploaded."""

import json
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(".build/package").resolve()))

from handler import handler
from release import RELEASE


class HandlerTest(unittest.TestCase):
    def test_http_response(self):
        response = handler({}, None)
        self.assertEqual(response["statusCode"], 200)
        self.assertEqual(response["headers"]["Content-Type"], "application/json")
        self.assertEqual(
            json.loads(response["body"]),
            {"message": "hello from Atmos", "release": RELEASE},
        )
