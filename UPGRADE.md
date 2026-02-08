# OSS to Pro Upgrade Architecture

This document outlines the upgrade path from Fusionaly OSS to Fusionaly Pro.

## Overview

Users start with the OSS version and can upgrade to Pro seamlessly:

1. **External manager** - `fusionaly-manager upgrade` swaps Docker image only
2. **Resume onboarding** - Pro detects existing OSS user and prompts for license
3. **Data preserved** - Same database, same settings, just adds Pro features

## Onboarding Flows

### OSS Onboarding (3 steps)
```
email → password → geolite → DONE → /admin/websites/new
```

### Pro Onboarding (5 steps)
```
email → password → geolite → license → openai → DONE → /admin/websites/new
```

### OSS-to-Pro Upgrade (2 steps)
When Pro detects an existing OSS installation:
```
(existing user) → license → openai → DONE → /admin/websites
```

## Detection Logic

Pro middleware checks on admin requests:

| Condition | Action |
|-----------|--------|
| No users exist | Fresh install → Full Pro onboarding (5 steps) |
| Users exist, no `license_key` setting | OSS upgrade → Resume from license step (2 steps) |
| Users exist, has `license_key` | Pro active → Normal operation |

```go
// Pro detection middleware
func ProOnboardingMiddleware(ctx *cartridge.Context) error {
    db := ctx.DB()

    hasUsers, _ := users.HasAnyUsers(db)
    if !hasUsers {
        // Fresh install - redirect to full Pro onboarding
        return ctx.Redirect("/setup")
    }

    hasLicense, _ := settings.GetSetting(db, "license_key")
    if hasLicense == "" {
        // OSS upgrade - redirect to Pro upgrade flow
        return ctx.Redirect("/pro/upgrade")
    }

    // Pro is active, proceed normally
    return ctx.Next()
}
```

## User Journey

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           OSS INSTALLATION                              │
├─────────────────────────────────────────────────────────────────────────┤
│  1. User runs: docker run fusionaly/fusionaly-oss                       │
│  2. Onboarding: email → password → geolite → DONE                       │
│  3. User creates website, uses Fusionaly OSS                            │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ User purchases Pro license
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     EXTERNAL MANAGER: UPGRADE                           │
├─────────────────────────────────────────────────────────────────────────┤
│  1. User runs: fusionaly-manager upgrade                                │
│  2. Manager stops OSS container                                         │
│  3. Manager pulls fusionaly/fusionaly-pro image                         │
│  4. Manager starts Pro container with same volumes                      │
│  (Manager only swaps image - license entry happens in browser)          │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                   PRO CONTAINER: FIRST REQUEST                          │
├─────────────────────────────────────────────────────────────────────────┤
│  Pro middleware detects:                                                │
│  - User exists (OSS onboarding completed)                               │
│  - No license_key setting (Pro not yet configured)                      │
│                                                                         │
│  → Redirects to /pro/upgrade (resume onboarding from license step)      │
│                                                                         │
│  Upgrade Flow:                                                          │
│  1. License key (enter/confirm from purchase)                           │
│  2. OpenAI API key (optional)                                           │
│  3. DONE → Redirect to dashboard with Pro features                      │
└─────────────────────────────────────────────────────────────────────────┘
```

## Settings Used

| Key | Description | Set By |
|-----|-------------|--------|
| `license_key` | Pro license key | Pro onboarding or upgrade flow |
| `license_email` | Email from license purchase | Pro onboarding or upgrade flow |
| `openai_api_key` | OpenAI API key for AI features | Pro onboarding or AI settings page |
| `geolite_license_key` | MaxMind GeoLite key | OSS or Pro onboarding |

## Architecture Boundaries

### fnctl (Inside Container)
- Database migrations
- User management (create-admin-user, change-password)
- Status checks
- Seeding test data

### External Manager (Outside Container)
- Docker image management (pull, swap OSS ↔ Pro)
- Container start/stop/restart
- Volume management
- Upgrade/downgrade operations
- **Does NOT handle license** - that's done in browser

### Pro Application (Inside Container)
- License validation (via license server API)
- License storage in database
- Pro onboarding/upgrade flow in browser

## Pro Feature Detection

Components check for Pro features availability:

```typescript
// Frontend - check if Lens/AI is available
const hasProFeatures = useMemo(() => {
  const licenseKey = settings?.find(s => s.key === 'license_key');
  return !!licenseKey?.value;
}, [settings]);

// Show Lens link only if Pro
{hasProFeatures && (
  <Link href={`/admin/websites/${websiteId}/lens`}>Lens</Link>
)}
```

## Pro Pages

Pro adds these pages:
- `/pro/upgrade` - License + OpenAI setup for OSS upgrades
- `/admin/websites/:id/lens` - AI-powered query interface
- `/admin/ai/settings` - AI configuration page
- Dashboard insights card - AI insights on dashboard

## Docker Compose Example

```yaml
# OSS installation
services:
  fusionaly:
    image: fusionaly/fusionaly-oss:latest
    volumes:
      - fusionaly-data:/app/storage

# After upgrade (same volumes, different image)
services:
  fusionaly:
    image: fusionaly/fusionaly-pro:latest
    volumes:
      - fusionaly-data:/app/storage
```

## Security Considerations

1. **License validation**: Validate with license server, cache result
2. **Offline mode**: Allow offline use with periodic revalidation
3. **Volume preservation**: Never delete user data during upgrade
4. **Rollback**: Keep OSS image available for downgrade if needed
