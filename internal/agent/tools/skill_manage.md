Write a skill: a short document of instructions that is loaded automatically in later sessions when its description matches what is being asked.

Use this when you have just worked out something the next session would otherwise have to work out again — a multi-step procedure specific to this project, a checklist that turned out to matter, the right way to drive an awkward tool here. A skill is for procedure. A single fact belongs in `memory`; code belongs in the repository.

`scope` decides who gets it:

- `project` — written under `.atlas/skills/` in this repository, so it travels with the code and applies to anyone working on it.
- `user` — written in the user's config directory, so it follows them across every project.

`action`:

- `create` — write a new skill. Fails if one of that name already exists.
- `update` — rewrite an existing one. Pass the full new instructions; this is not a patch.
- `delete` — remove one.

`name` is lowercase words joined by hyphens and is also the directory name. `description` is the only thing a future session sees before deciding to load the skill, so write it as a trigger — when to reach for this — not as a title.

Keep `instructions` short and imperative. A skill that restates what a competent reader already knows costs a session tokens and teaches it nothing.

Every write is shown to the user for approval. A new skill is available in the next session, not this one.
