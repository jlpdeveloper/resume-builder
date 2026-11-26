# AI guide: authoring résumé YAML

Use this guide when creating or editing résumé data. Keep verified career facts in the profile, select and order that information in a résumé definition, and leave visual styling to the template. Never invent dates, outcomes, metrics, credentials, links, or responsibilities; when a fact is missing, omit it or ask the user.

For CLI commands and PDF review, follow the human workflow in [README.md](README.md#for-humans).

## File locations and responsibilities

Use the repository's existing conventions:

```text
profile.yml                 reusable career facts and contact information
bases/<name>.yml            shared selections and defaults
resumes/<name>.yml          one résumé's target and content selections
templates/<name>.yml        page, font, color, spacing, and layout settings
output/                     generated PDFs and manifests
```

The CLI accepts explicit file paths and does not discover files by directory. References such as `profile`, `base`, and `template` are paths relative to the YAML file that contains the reference. Put each YAML document's common envelope at the top and use the matching kind:

```yaml
schema: resume/v1
kind: profile # profile, base, resume, or template
id: alex-morgan
```

Give every document a stable ID; document IDs must not collide within a dependency graph. Give profile entries stable IDs too, because bases and résumé definitions select those entries by ID. Keep IDs unique within their collection and use simple lowercase hyphenated names.

### Profile: source facts

Keep reusable, candidate-specific facts in `profile.yml`:

- `header`: name, contact details, links, and credentials shown with contact information.
- `employment`: employers (`id`, `name`, optional location/link) containing roles (`id`, `title`, `start`, optional `end`) and factual `accomplishments`.
- `skills`: named groups with stable IDs and skill lists.
- `projects`: named work with optional role, dates, link, technology stack, and accomplishments.
- `education` and `certifications`: academic and professional credentials, respectively.
- `summaries`: reusable professional summaries; `impact`: reusable cross-cutting impact statements.

Keep each accomplishment under the role or project where it happened. Use dates as quoted strings, preferably `"YYYY-MM"` (for example, `"2022-01"`), and leave an ongoing role's end date empty rather than guessing.

### Base: shared defaults

Put a base in `bases/` when multiple résumé definitions should share a profile, template, page limit, section order, or content defaults. A base can contain default employer/role entries under `employment` and skill groups under `skills`; its `projects`, `education`, and `certifications` lists reference profile entry IDs. Use `bases/staff-engineer.yml` as a reference for the current format. Keep presentation settings in the referenced template, not in the base.

### Resume: tailor and select

Put each per-application definition in `resumes/`. It references its profile and optionally a base and template. Use its `target` for application context and its optional `headline` only for a concise, supported positioning statement. These fields are not substitutes for career facts.

Use the résumé fields to select and order profile content:

- `summary`: one profile summary `id`, or a truthful inline `text`.
- `impact`: ordered profile impact IDs, or inline statements when needed.
- `experience`: employers by ID, with selected `roles` and optionally selected accomplishment `bullets`.
- `skills`: groups by profile `group` ID, optionally narrowed to a subset of that group's skills.
- `projects`, `education`, `certifications`: ordered lists of profile entry IDs.
- `sections`: the order in which the selected content is displayed.
- `pageLimit`, `metadata`, and `output`: per-résumé build settings where needed.

List employers newest first and roles within each employer newest first. Resume selection list order is retained in the resolved résumé. When a non-empty résumé selection is provided for experience, skills, projects, education, or certifications, it replaces that category's base defaults rather than appending to them. If `sections` is omitted, the base order is used, or the built-in order when there is no base order. Section order does not select content by itself: select the items and include the corresponding section.

An abbreviated example:

```yaml
schema: resume/v1
kind: resume
id: alex-platform-role
profile: ../profile.yml
base: ../bases/staff-engineer.yml

target:
  company: Example Company
  role: Staff Platform Engineer

summary:
  id: platform-leader
impact:
  - id: deployment-reliability
experience:
  - company: northstar
    roles:
      - staff-platform
      - senior-platform
skills:
  - group: platform
    skills:
      - Go
      - Kubernetes
projects:
  - id: delivery-platform
sections:
  - summary
  - impact
  - experience
  - skills
  - projects
  - education
  - certifications
```

## Map source facts to résumé sections

Use the section names exactly as written; the selected section list determines their display order.

| Section | Include |
|---------|---------|
| `summary` | A short, general professional overview relevant to the résumé; keep it distinct from individual accomplishments. |
| `impact` | A small set of high-value results that demonstrate impact across roles or projects. Avoid repeating the same accomplishment verbatim in experience. |
| `experience` | Paid or formal roles, grouped by employer, with dates and role-specific accomplishment bullets. |
| `skills` | Concise, evidence-backed tools and capabilities grouped by area. Do not add a skill solely because it appears in the job description. |
| `projects` | Significant independent, open-source, or product work that merits its own entry; include role, dates, technology, and outcomes when known. |
| `education` | Degrees and other academic study. |
| `certifications` | Issued professional credentials, including issuer and dates when known. |

Keep each section focused: use experience for work history, projects for distinct project entries, and impact for results that would otherwise be hard to find across roles. Preserve the user's wording and numbers unless asked to edit them. Do not duplicate the same full content in multiple sections just to fill space.

## Validate authored YAML

Before handing back a YAML change, validate the résumé path and inspect its resolved output. Confirm that every selected ID exists, the selected lists and section order match the request, and the final PDF contains only the intended content. Refer to the README's human workflow for exact commands and visual review steps.