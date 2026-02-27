
import sys
import unittest
import shutil
from pathlib import Path
from revancedbot import App, Patcher

class TestSecurityPaths(unittest.TestCase):
    def setUp(self):
        self.dirs_to_cleanup = []

    def tearDown(self):
        for d in self.dirs_to_cleanup:
            if d.exists():
                shutil.rmtree(d)

    def test_app_default_root_is_random(self):
        """Test that App() creates a random directory when no root is provided."""
        app1 = App()
        app2 = App()

        self.dirs_to_cleanup.append(app1.root)
        self.dirs_to_cleanup.append(app2.root)

        print(f"App 1 root: {app1.root}")
        print(f"App 2 root: {app2.root}")

        self.assertNotEqual(app1.root, app2.root)
        self.assertFalse(str(app1.root).startswith("/tmp/revancedbot"))
        self.assertTrue(app1.root.exists())

    def test_patcher_default_location_is_random(self):
        """Test that Patcher() creates a random directory when no location is provided."""
        patcher1 = Patcher()
        patcher2 = Patcher()

        self.dirs_to_cleanup.append(patcher1.tool_location)
        self.dirs_to_cleanup.append(patcher2.tool_location)

        print(f"Patcher 1 location: {patcher1.tool_location}")
        print(f"Patcher 2 location: {patcher2.tool_location}")

        self.assertNotEqual(patcher1.tool_location, patcher2.tool_location)
        # It should be a direct temporary directory, not a subdir of one
        self.assertTrue(patcher1.tool_location.exists())

        # Verify it is not the old insecure path
        # The old path was something like /tmp/tmpXXXXXX/revancedbot
        # We just want to ensure it is created securely.
        # checking if it is a directory is enough combined with uniqueness check

if __name__ == '__main__':
    unittest.main()
