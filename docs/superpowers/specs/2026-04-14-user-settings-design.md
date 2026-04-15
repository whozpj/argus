# User Settings — Display Name

**Date:** 2026-04-14
**Status:** Approved

## What We're Building

A minimal user settings feature: users can set a display name that replaces their email address in the dashboard header. Access via a dropdown on the header name/email element.

## Scope

In scope:
- `display_name` field on the user record (nullable, max 50 chars)
- `PATCH /api/v1/me` endpoint to update it
- Header dropdown (Settings + Sign out) replacing the bare sign-out button
- `/settings` page with a single display name form field

Out of scope: avatar, password, email change, account deletion, notification preferences.

## Database

Add column to `users` table in the existing schema SQL:

```sql
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT;
```

Applied on server startup via the schema migration block (same pattern as existing tables).

## Server

### `GET /api/v1/me` (existing)
Add `display_name` to response — null if unset, string if set.

```json
{
  "id": "user-123",
  "email": "alice@example.com",
  "display_name": "Alice",
  "projects": [...]
}
```

### `PATCH /api/v1/me` (new)
- Auth: JWT required (`RequireJWT` middleware)
- Request body: `{"display_name": "Alice"}`
- Validation: non-empty string, max 50 characters
- Returns: updated user object (same shape as `GET /api/v1/me`)
- Errors: 400 if validation fails, 401 if no JWT

### Store

New method `UpdateDisplayName(userID, displayName string) error` on `*DB`.

## UI

### Header — name/email → dropdown

Replace the current bare "Sign out" button with a `DropdownMenu` trigger showing:
- Display name if set, otherwise email (truncated)
- Dropdown items: **Settings** (links to `/settings`), **Sign out**

Uses shadcn `DropdownMenu` component.

### `/settings` page

Client component. On mount:
1. Reads JWT from localStorage, redirects to `/login` if missing
2. Calls `fetchMe()` to get current `display_name`
3. Pre-fills the input

Form:
- **Display name** — text input, max 50 chars, placeholder "Your name"
- **Save** button — calls `PATCH /api/v1/me`, shows inline success/error message
- Below form: "Signed in as `email`" shown read-only

After successful save, `display_name` in the header updates on next navigation (no need for global state — Next.js router refresh is sufficient).

### `lib/api.ts` additions

```typescript
export async function updateDisplayName(displayName: string): Promise<MeResponse>
```

### `lib/types.ts` addition

Add `display_name: string | null` to `MeResponse`.

## Data Flow

```
Header click → DropdownMenu
  → "Settings" → navigate to /settings
  → "Sign out" → clear token → /login

/settings mount → fetchMe() → pre-fill input
User types → Save → updateDisplayName() → PATCH /api/v1/me
  → success: show "Saved" message
  → error: show error message
Next visit to dashboard: fetchMe() returns updated display_name → shown in header
```

## Error Handling

- Empty display name → 400 from server → "Display name cannot be empty" shown inline
- Name > 50 chars → validated client-side before submit
- Network error → "Failed to save, please try again" shown inline
- Unauthenticated → 401 → redirect to `/login`

## Testing

### Server (Go)
- `TestUpdateDisplayName_Valid` — updates and returns updated user
- `TestUpdateDisplayName_Empty` — returns 400
- `TestUpdateDisplayName_TooLong` — returns 400 (>50 chars)
- `TestUpdateDisplayName_Unauthenticated` — returns 401

### Playwright (e2e)
- Settings page renders with pre-filled display name
- Empty save shows validation error
- Valid save shows success message
- Header shows display name after save + navigation
