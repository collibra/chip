# Glossary: "what is ...?"

The customer can ask "what is ...?" about anything at any time. Answer with the line below, in your own voice, then return to where you were. A product term the customer needs appears once with its short explanation; after that, use the term alone. Never explain beyond these lines how Guardian works inside (`refusal-scripts.md › R-INTERNALS`).

| Term | Answer |
|---|---|
| Collibra | "Collibra is where your company keeps track of its data and AI, and the rules for using them." |
| AI Command Center | "The part of Collibra for AI: your AI agents, models, AI use cases and their rules." |
| Guardian | "Guardian watches what an AI agent does while it runs, checks it against your agent contracts, and records any issues in Collibra." |
| Agent contract | "A list of rules for AI agents, written in plain language, such as "never share a customer's card number". One can apply to every agent; an agent can also have its own." |
| AI use case | "A description of what an AI agent is for, who uses it, and its risks." |
| Rules file | "The file Collibra makes from your agent contracts. Your agent reads it when it starts, so Guardian knows what to check." The documentation calls it the "enforcement bundle". |
| Monitor (record only) | "Guardian records what it would have stopped, without stopping anything. You can switch to stopping it at any time." |
| Enforce (stop it) | "Guardian applies your rules: it blocks the action, or changes it (for example, hides sensitive data), as your rules say. Your users see the effect straight away." |
| Collecting payloads | "Keeping the conversation behind a flagged action, so you can see why it was flagged. Your company decides whether Collibra keeps it." |
| Collibra Edge | "The Collibra component your administrator runs to connect Collibra to systems such as Databricks." |
| Guardian SDK | "The Python package that lets Guardian check your agent. Collibra shares it with you." |
| Agent scan | "A description of your agent's models, instructions and tools, read from its code. Collibra uses it to know your agent and to notice when it changes." |
| Apply latest rules | "The button on your agent's **Contract** tab in Collibra that turns the agent contracts into a rules file for the agent's newest version. I can do the same for you." |
| Allow, Flag, Deny, Escalate, Rewrite | "The documentation's names for what Guardian decided about an action. The page on how Guardian works explains each one." Then the link for that topic from `references/documentation.md`. |
