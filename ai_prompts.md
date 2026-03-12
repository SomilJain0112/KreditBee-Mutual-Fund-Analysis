# AI Prompt Log

This document records the prompts used during the development of the **Mutual Fund Analytics** project.

---

## Prompt 1 — High Level Design

```text
product_documentation.pdf is the source of truth that we will use to make project

Generate the complete high level design how structure ,api flow etc gonna look like.
Mention complete flow of each api.
Mention what other tools we are gonna need and use for project like SQLite etc
Mention all the outcomes that @Assignment-2_ Mutual Fund Analytics (1).pdf is expecting
Give how project structure should look like

Also mention what design decision you made and why?

Follow @.cursor/BestCodingPractise.md for best practises but its not mandatory.
```

---

## Prompt 2 — Architecture Improvement

```text
I have a suggestion use gin and tables should have proper normalisation and no redundancy
```

---

## Prompt 3 — Implementation Instructions

```text
@/Users/somiljain/.cursor/plans/mutual_fund_analytics_hld_e76949b4.plan.md looks perfect to me
Start implementing it.

First of all call endpoints of https://api.mfapi.in/mf that we are gonna use to see actual response before starting implementation.

After that create directory structure and boilerplate code and setup sqlite and make apis one by one end to end.

After creating each api check if its working as expected. Handle gracefully edge cases.

Write its testcases and edge cases and run it as well to see everything is working fine.

Then create a README.md and document how to do complete setup very easily and all the apis that are implemented. Mention all curls and their responses.
```

---

## Prompt 4 — README Generation

```text
In my last prompt I had mentioned about README.md but its not generated yet.

Do it create a README.md and document how to do complete setup very easily and all the apis that are implemented. Mention all curls and their responses.

Also check all those curls are working as expected check their response as well.
```

---

## Prompt 5 — Documentation Formatting

```text
I just saw README and @DESIGN_DECISIONS.md is looking so ugly
Make it beautiful using below

Formatting Requirements:

- Use clear Markdown headings
- Use tables where appropriate
- Use JSON code blocks
- Use curl code blocks
- Make the document readable and professional
- Ensure the README looks like a real open-source backend service documentation
```