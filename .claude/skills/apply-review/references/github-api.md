# GitHub review-thread API

Copy-paste exact. Every command below was verified against the live schema.

Set once:

```bash
OWNER=$(gh repo view --json owner --jq .owner.login)
NAME=$(gh repo view --json name --jq .name)
```

## Argument typing

`gh api graphql` passes `-f` values as **strings** and `-F` values as typed JSON.
A GraphQL `Int!` argument given with `-f` fails with a type error, so pull-request
numbers always use `-F`.

## Pagination is the trap that loses comments

`pullRequests(first: 25, orderBy: {field: CREATED_AT, direction: DESC})` returns
the 25 newest and silently omits everything older. A stack of sixteen sitting
below a batch of newer pull requests reports zero threads and looks clean.

Either page until `hasNextPage` is false, or resolve the stack map first and query
those exact numbers. Prefer the second: one round trip per pull request, and it
cannot silently truncate.

## Resolve the stack

Every open pull request with its base, head, and body. The body carries the stack
map that `stacked-pr` writes.

```bash
gh api graphql --paginate -f owner="$OWNER" -f name="$NAME" -f query='
query($owner:String!,$name:String!,$endCursor:String){
  repository(owner:$owner,name:$name){
    pullRequests(states:OPEN, first:100, after:$endCursor){
      pageInfo{ hasNextPage endCursor }
      nodes{ number isDraft baseRefName headRefName title body }
    }
  }
}'
```

`--paginate` requires the cursor variable to be named `endCursor`.

## Everything on one pull request

Threads, review states, and top-level comments in a single query.

```bash
gh api graphql -F number=24 -f owner="$OWNER" -f name="$NAME" -f query='
query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){
    pullRequest(number:$number){
      headRefOid
      reviews(first:50){ nodes{ state author{login} submittedAt } }
      comments(first:50){ nodes{ databaseId url author{login} body } }
      reviewThreads(first:100){
        pageInfo{ hasNextPage endCursor }
        nodes{
          id isResolved isOutdated path line originalLine startLine diffSide
          comments(first:20){
            nodes{ databaseId url author{login} state body createdAt diffHunk }
          }
        }
      }
    }
  }
}'
```

Flatten to one line per thread:

```bash
--jq '.data.repository.pullRequest.reviewThreads.nodes[]
  | "\(.id)\t\(.isResolved)\t\(.path):\(.line)\t\(.comments.nodes[0].url)\t\(.comments.nodes[0].state)\t\(.comments.nodes[0].body | gsub("\n";" | "))"'
```

## Reply to a thread

```bash
gh api graphql -f threadId="PRRT_xxx" -f body="Fixed in ..." -f query='
mutation($threadId:ID!,$body:String!){
  addPullRequestReviewThreadReply(input:{
    pullRequestReviewThreadId:$threadId, body:$body
  }){ comment{ url } }
}'
```

REST fallback, using the first comment's `databaseId`:

```bash
gh api -X POST "repos/$OWNER/$NAME/pulls/24/comments/3835629093/replies" -f body="..."
```

## Resolve a thread

```bash
gh api graphql -f threadId="PRRT_xxx" -f query='
mutation($threadId:ID!){
  resolveReviewThread(input:{threadId:$threadId}){ thread{ id isResolved } }
}'
```

Read `thread.isResolved` back. A mutation that returns without it true has
resolved nothing, and reporting it as resolved breaks the one guarantee the
resolved state exists to give.

## Post or edit the Fix Audit comment

Review threads and top-level comments are different objects. A top-level comment
is an issue comment, so it is addressed through the `issues` endpoints even on a
pull request.

```bash
# find an existing one
gh api "repos/$OWNER/$NAME/issues/15/comments" --paginate \
  --jq '.[] | select(.body | startswith("# Fix Audit")) | .id'

# create
gh pr comment 15 --body-file audit.md

# edit in place
gh api -X PATCH "repos/$OWNER/$NAME/issues/comments/<id>" -F body=@audit.md
```

## Field notes

| Field                                 | What it gives you                                                                                                                                                                                          |
| ------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `reviewThread.id`                     | `PRRT_…` node id. The only thing `resolveReviewThread` accepts, and not derivable from a REST comment id.                                                                                                  |
| `comment.url`                         | The thread permalink, `…/pull/24#discussion_r3835629093`. Use it directly, never hand-build it.                                                                                                            |
| `comment.databaseId`                  | The numeric id REST uses, for the reply fallback above.                                                                                                                                                    |
| `comment.state`                       | `SUBMITTED` or `PENDING`. A pending comment belongs to an unsubmitted review, is visible only to its author, and cannot be replied to.                                                                     |
| `reviewThread.isOutdated`             | True only when the line moved on **that** pull request's head. Fixes land on the merge vehicle, so a slice thread stays `false` forever. It is not an addressed signal.                                    |
| `reviewThread.line` vs `originalLine` | `line` is the position on that pull request's current head, `originalLine` where the comment was first placed. Both are positions in that pull request's diff. Neither is a position on the merge vehicle. |
| `pullRequest.reviews` empty           | Nobody submitted a review. Distinct from a submitted review that found nothing, which reports a review carrying zero comments.                                                                             |

## Draft status is irrelevant

Draft pull requests carry review threads exactly as any other does. A draft
reporting no threads has no threads.
