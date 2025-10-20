# Mermaid Diagrams for GitHub Authentication

## Overview

5 Mermaid diagrams for GitHub User and GitHub App provider documentation. These visual aids help users understand GitHub-specific authentication flows and architecture.

---

## GitHub User Provider (`github/user`)

### 1. Device Flow Sequence Diagram

**Purpose:** Show the OAuth Device Flow authentication process

```mermaid
sequenceDiagram
    participant User
    participant Atmos
    participant GitHub
    participant Keychain as OS Keychain

    User->>Atmos: atmos auth login
    Atmos->>Keychain: Check for cached token

    alt Token exists and valid
        Keychain-->>Atmos: Return token
        Atmos-->>User: ✓ Already authenticated
    else Token missing or expired
        Atmos->>GitHub: Request device code
        GitHub-->>Atmos: device_code, user_code, verification_uri
        Atmos-->>User: Visit https://github.com/login/device<br/>Enter code: ABCD-1234
        User->>GitHub: [Opens browser, enters code]
        GitHub-->>User: Authorize app
        User->>GitHub: Confirm authorization

        loop Poll for token
            Atmos->>GitHub: Check authorization status
            GitHub-->>Atmos: pending...
        end

        GitHub-->>Atmos: access_token (8h validity)
        Atmos->>Keychain: Store token securely
        Atmos-->>User: ✓ Successfully authenticated
    end
```

**Used in:** GitHub User provider docs, Overview section

---

### 2. Provider Selection Decision Tree

**Purpose:** Help users choose between `github/user` and `github/app`

```mermaid
graph LR
    A[Need GitHub Access?] --> B{Interactive User?}
    B -->|Yes| C[github/user]
    B -->|No| D[github/app]

    C --> E[Local Development]
    C --> F[Manual Operations]
    C --> G[Personal Workflows]

    D --> H[CI/CD Pipelines]
    D --> I[Automation]
    D --> J[Bots]

    style C fill:#4CAF50
    style D fill:#2196F3
```

**Used in:** "When to Use" section for both providers

---

### 3. Token Lifecycle State Diagram

**Purpose:** Show token states and expiration handling

```mermaid
stateDiagram-v2
    [*] --> NoToken: First use
    NoToken --> Authenticating: atmos auth login
    Authenticating --> Active: Device Flow success
    Active --> Active: Token valid (< 8h)
    Active --> Expired: Time passes (> 8h)
    Active --> InUse: atmos auth exec
    InUse --> Active: Command completes
    Expired --> Authenticating: Auto-refresh
    Authenticating --> [*]: Auth failed

    note right of Active
        Token stored in
        OS keychain
    end note

    note right of Expired
        User prompted to
        re-authenticate
    end note
```

**Used in:** GitHub User provider docs, Token Lifecycle section

---

### 4. Multiple GitHub Accounts Architecture

**Purpose:** Show how to manage separate work and personal accounts

```mermaid
graph TB
    subgraph "OS Keychain"
        K1[atmos-github-work<br/>Token: ghu_work123...]
        K2[atmos-github-personal<br/>Token: ghu_pers456...]
    end

    subgraph "Atmos Configuration"
        P1[Provider: github-work<br/>Client ID: Iv1.work123<br/>Keychain: atmos-github-work]
        P2[Provider: github-personal<br/>Client ID: Iv1.personal456<br/>Keychain: atmos-github-personal]

        I1[Identity: work<br/>via: github-work]
        I2[Identity: personal<br/>via: github-personal]

        P1 --> I1
        P2 --> I2
    end

    I1 -.->|reads| K1
    I2 -.->|reads| K2

    U1[atmos auth exec -i work<br/>-- gh repo list cloudposse] --> I1
    U2[atmos auth exec -i personal<br/>-- gh repo list myusername] --> I2

    style K1 fill:#FFE082
    style K2 fill:#FFE082
    style I1 fill:#81C784
    style I2 fill:#81C784
```

**Used in:** GitHub User provider docs, Example 2 (Multiple Accounts)

---

## GitHub App Provider (`github/app`)

### 5. GitHub App JWT Flow Sequence Diagram

**Purpose:** Show how GitHub App authentication works with JWT signing

```mermaid
sequenceDiagram
    participant User
    participant Atmos
    participant File as Private Key File/Env
    participant GitHub as GitHub API

    User->>Atmos: atmos auth exec -i bot
    Atmos->>File: Read private key
    File-->>Atmos: PEM private key

    Note over Atmos: Generate JWT<br/>(signed with private key)<br/>Valid for 10 minutes

    Atmos->>GitHub: POST /app/installations/{id}/access_tokens<br/>Authorization: Bearer {JWT}
    GitHub-->>Atmos: Installation access token (1h validity)

    Note over Atmos: Cache token in memory

    Atmos->>Atmos: Set GITHUB_TOKEN env var
    Atmos->>User: Execute command with token

    alt Token expired (after 1h)
        User->>Atmos: Next command
        Atmos->>Atmos: Regenerate JWT and request new token
        Atmos->>GitHub: POST /app/installations/{id}/access_tokens
        GitHub-->>Atmos: New installation token
    end
```

**Used in:** GitHub App provider docs, Overview section

---

## Color Conventions

### Component Types
- **Green (#4CAF50)**: User-facing components, manual workflows
- **Blue (#2196F3)**: Automation components, CI/CD
- **Yellow (#FFE082)**: Storage/persistence (keychains)
- **Light Green (#81C784)**: Identities

### States
- **Green**: Active/success states
- **Yellow**: Intermediate states
- **Red**: Error/failed states

---

## Diagram Types Used

### For GitHub Authentication

1. **Sequence Diagrams** (`sequenceDiagram`)
   - Best for: Time-based authentication flows
   - Used in: Device Flow, JWT signing flow

2. **Decision Trees** (`graph LR`)
   - Best for: Choosing between providers
   - Used in: User vs App selection

3. **State Diagrams** (`stateDiagram-v2`)
   - Best for: Token lifecycle management
   - Used in: Token expiration and refresh

4. **Architecture Diagrams** (`graph TB`)
   - Best for: System architecture, multiple accounts
   - Used in: Account isolation, component relationships

---

## Docusaurus Integration

Mermaid diagrams work natively in Docusaurus:

```markdown
\`\`\`mermaid
graph LR
    A --> B
\`\`\`
```

No additional configuration required.

---

## Testing Diagrams

Verify diagrams before committing:

1. **Mermaid Live Editor**: https://mermaid.live - paste and preview
2. **Local Docusaurus**: `cd website && npm run start`
3. **Build check**: `cd website && npm run build`

---

## Usage in Documentation

### GitHub User Provider Page

```markdown
## Overview

[Intro text about Device Flow...]

### Authentication Flow

\`\`\`mermaid
sequenceDiagram
    [Device Flow diagram]
\`\`\`

### When to Use

\`\`\`mermaid
graph LR
    [Decision tree diagram]
\`\`\`

## Token Lifecycle

\`\`\`mermaid
stateDiagram-v2
    [Lifecycle diagram]
\`\`\`

## Examples

### Multiple Accounts

\`\`\`mermaid
graph TB
    [Architecture diagram]
\`\`\`
```

### GitHub App Provider Page

```markdown
## Overview

[Intro text about GitHub Apps...]

### Authentication Flow

\`\`\`mermaid
sequenceDiagram
    [JWT flow diagram]
\`\`\`

### When to Use

\`\`\`mermaid
graph LR
    [Same decision tree as User provider]
\`\`\`
```

---

## Summary

- **5 diagrams total**: 4 for User provider, 1 for App provider (plus shared decision tree)
- **Focused scope**: GitHub authentication only
- **Visual consistency**: Same color scheme and styling
- **Practical value**: Each diagram solves a specific user question
- **Maintainable**: Text-based, version-controlled diagrams

All diagrams are implementation-ready and focused on the GitHub authentication providers being delivered in this PR.
