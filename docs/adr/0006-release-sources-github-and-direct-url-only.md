# Release Sources: GitHub API and Direct URL only, no scraping

AiM supports two Release Sources: GitHub repositories, through the Releases API, and Direct URLs, where changes are detected from HTTP `ETag`/`Last-Modified`/`Content-Length` headers. It never scrapes HTML release pages, which break without warning. Any GitHub link, including a direct asset download URL, resolves to its repository. A direct link also supplies the installed Version and the Asset Pattern. Every source sits behind one interface, so GitLab or Codeberg can be added later.

## Consequences

- Direct URL apps have no comparable Version, only "changed or not", so a Rollback can't be followed by a "newer" check in the usual sense.
- Unauthenticated GitHub calls are limited to 60 an hour. An optional token in config covers heavy users.
