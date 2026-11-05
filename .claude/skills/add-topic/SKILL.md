---
name: add-topic
description: Create a FIRe Lab pack — a topic of lessons with practice questions — when the learner asks for lessons, a topic or practice on something ("составь уроки по …", "make me a topic on …", "I struggle with …, give me exercises"). Covers planning from easy to hard, writing lessons and questions, linking to existing ideas with encompassing weights, and validating.
---

# Add a topic (pack)

The result is a folder `packs/<id>/` that `./fire validate <id>` accepts. The app picks it up by itself.

## 1. Pin down the request

From the learner's words take: the subject, how many lessons (default 5–7), how many questions per lesson (default 5–6), the level (default: from basics to genuinely hard), the language (default: the learner's). Ask only if the subject itself is unclear.

## 2. Look at what exists

- `./fire search <subject words>` (try 2–3 phrasings, and the words in the pack language). Note codes of ideas the new lessons build on or exercise.
- `./fire show <code>` for the closest hits, to see their level and wording.
- If an existing pack of the learner covers this subject, add a topic to it instead of a new pack. Otherwise create a new pack: `./fire new <id> -title "…"`, with a short id like `quantifiers`.

## 3. Plan

Write down the lessons in order, each one idea with one skill. A pack with more than about six topics groups them into `categories` (big areas like «Линейная алгебра»); a small pack does not need them. Go from the simplest form to the hardest, so each lesson builds on the previous ones. A good pack has a clear ramp: recognise → use → transform → combine → nasty cases. Each idea in the topic usually `requires` the previous one (key), and `uses` earlier ones it exercises.

## 4. Write lessons

Per lesson (Markdown, 300–8000 characters; for long ones use `lesson_file: "lessons/<idea>.md"`):

1. The meaning in plain words first — what problem the idea solves.
2. The formula or rule, every symbol explained.
3. Why it works / intuition, and the typical mistake.
4. One short worked example, carried to the end.

Math in KaTeX: `$…$` inline, `$$…$$` display. In `pack.json` strings every backslash is doubled (`\\forall`); in `.md` lesson files it is single (`\forall`). `./fire validate` catches unbalanced `$` and single backslashes that JSON turned into control characters.

## 5. Write questions

3–8 per lesson (follow the learner's number). Each question:

- tests understanding or skill, not recall of the lesson's wording; vary the kinds: read/translate, decide which is stronger, transform (negate, simplify), find a counterexample, spot the error;
- has exactly one correct option and 3–5 plausible wrong ones built from real mistakes;
- has an `explanation`: why the right answer is right and why the tempting wrong one is wrong;
- gets `difficulty` 1–3, rising through the lesson; the last lesson's last questions should be really hard;
- puts the correct option at different positions across questions (`correct` is a 0-based index).

## 6. Link and weigh

`requires` — the lesson cannot be followed without that idea (gates it); `uses` — the lesson exercises it (credit only). Weights `w` 0.05–1 by the rubric in [`docs/pack-format.md`](../../../docs/pack-format.md#weights-requires-and-uses): 1.0 fully performs it, 0.7–0.9 central in almost every question, 0.4–0.6 substantial part, 0.1–0.3 touches it. Link only the most direct ideas, not their prerequisites. Link to other packs mostly with `uses`.

## 7. Check yourself

- Solve every question without looking at `correct`, then compare. Recompute every number. Fix any question with two defensible answers.
- `./fire validate <id>` — repeat until there are no errors; fix the warnings too where they make sense.

## 8. Report

Tell the learner: the pack id and title, the list of lessons, that it is already in the app (choose it in the program switch on the main page, or pin a topic with “↑ первой”), and anything you were unsure about.
