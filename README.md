# micb-go-cli

CLI for MICB (maib) mobile banking.

## Build

Requires Go 1.21+.

```sh
go build -o micb-go-cli .
```

## Usage

Run the binary:

```sh
./micb-go-cli
```

### Commands

| Command | Description |
|---------|-------------|
| `login` | Login with password and captcha |
| `mma-setup` | Set up MMA profile and PIN |
| `pin-login` | Quick login with PIN |
| `logout` | Logout and clear session |
| `user` | User profile info |
| `accounts` | List accounts |
| `cards` | List cards |
| `card-info [N]` | Card details |
| `card-number [N]` | Show full card number (active cards only) |
| `notifications` | List notifications |
| `captcha` | Download captcha image |
| `device` | Emulate device verification |
| `devices` | List registered mobile devices |
| `device-use <N>` | Switch active device ID |
| `status` | Check session status |
| `help` | Show help |

Aliases: `acc` for `accounts`, `me` for `user`, `notif` for `notifications`, `q` for `exit`.

### First-time setup

1. Run `login` -- enter your internet banking credentials and captcha.
2. Run `mma-setup` -- creates a MMA profile and sets a 4-digit PIN for quick access.
3. Subsequent sessions can use `pin-login <PIN>` instead of full login.

### Card numbers

Active cards only. Run `cards` to list cards, then `card-number <N>` to fetch the full PAN.

## Configuration

Session data and MMA profile are stored in `~/.micb-cli/`.
