# One systemd user timer per Managed App

Scheduled Checks run from a systemd user template unit, `aim-check@.timer`, with one instance per Managed App and that app's Poll Rate in a drop-in `OnCalendar=`. A Poll Rate of "never" means the app has no timer instance. AiM runs no daemon, and the tray app never schedules anything.

## Considered Options

- **A long-running daemon / the tray app polls**: updates would stop whenever the GUI isn't running.
- **A single daily timer that checks whichever apps are due**: simpler, but apps aren't individually visible in `systemctl --user list-timers` and can't be scheduled independently.

## Consequences

- `Persistent=true` catches up on Checks missed while the machine was off; `RandomizedDelaySec` spreads them out after login.
- Unmanage and Remove must stop and disable the app's timer instance and delete its drop-in. Uninstall also deletes the template, so no AiM units are left behind.
- Poll Rates are daily, weekly, monthly or never. Hourly was rejected as unnecessary.
