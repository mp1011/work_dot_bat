---
status: In Progress
---

# GOAL
Contain all configuration that work.bat needs for a project inside projects.json.

Currently we put the test commands in a separate json, but we want all configuration to be available in one place.

# Test Layers
- UltraLight: linters, type checkers. Cheap tests that we want to run often
- Light: unit tests. Tests we want to run after a code change.
- Heavy: end to end tests, integration tests.  Tests we want to run after whole feature is ready.
- UltraHeavy: expensive quality gates, these are tests we want to run before pushing any code to origin

# JSON Structure
```
{
    "name": "my_project_name",
    "path": "C:\\path\to\project"
    "tests":
    {
        ultra_light: ["cmd", "arg1", "arg2"],
        light: ["cmd", "arg1", "arg2"],
        heavy: ["cmd", "arg1", "arg2"],
        ultra_heavy: ["cmd", "arg1", "arg2"],       
    }
}
```