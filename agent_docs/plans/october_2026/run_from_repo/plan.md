---
status: Done
---

# GOAL

Allow a developer to open a session from the project repository, rather than launching work.bat from here.

# Tasks
- provide some way, within this work_dot_bat project, to write a wrapper command in the project repository which calls ./bin/work for that project
    - do this upon creating a new project, but also provide some way to add it to existing projects
- the file should be git ignored as it depends on local paths
- from the project repo, the developer should be able to launch the work tool via .tools/work
- same thing for auto_daemon and prepare_for_push
