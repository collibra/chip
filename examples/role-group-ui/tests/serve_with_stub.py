"""Serves the UI on port 8766 with the stub in place of Claude, for browser checks."""
import os, pathlib, sys
os.environ["PORT"] = "8766"
sys.path.insert(0, str(pathlib.Path(__file__).parent.parent))
import server
server.CLAUDE_BIN = str(pathlib.Path(__file__).with_name("fake_claude.py"))
server.main()
