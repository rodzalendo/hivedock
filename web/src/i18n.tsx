/* eslint-disable react-refresh/only-export-components -- this module intentionally
   exports the provider component alongside the useI18n hook and locale data. */
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";

// Lightweight, dependency-free i18n (the project keeps its dependency list
// tight). Strings live in the dictionaries below keyed by dot-path; t() looks
// up the active language, falls back to English, then to the key itself, and
// interpolates {{vars}}. The choice is remembered per browser.

export type Lang = "en" | "pl" | "de" | "es" | "fr";

export const LANGUAGES: { id: Lang; label: string }[] = [
  { id: "en", label: "English" },
  { id: "pl", label: "Polski" },
  { id: "de", label: "Deutsch" },
  { id: "es", label: "Español" },
  { id: "fr", label: "Français" },
];

type Dict = Record<string, string>;

const en: Dict = {
  "nav.home": "Home",
  "nav.stacks": "Stacks",
  "nav.updates": "Updates",
  "nav.settings": "Settings",
  "sidebar.backendOk": "Backend ok",
  "sidebar.backendDown": "Backend unreachable",
  "sidebar.signOut": "Sign out",

  "common.save": "Save",
  "common.saving": "Saving…",
  "common.cancel": "Cancel",
  "common.active": "Active",
  "common.remove": "Remove",

  "settings.title": "Settings",
  "settings.appearance": "Appearance",
  "settings.appearanceHelp":
    "The theme is saved in this browser only. It isn't shared with other users or devices.",
  "settings.language": "Language",
  "settings.languageHelp": "The interface language, saved in this browser.",
  "settings.autoUpdate": "Automatic update check",
  "settings.autoUpdateHelp":
    "How often HiveDock checks registries for newer images in the background. Changes apply within a minute, no restart needed.",
  "settings.autoUpdateSaved": "Saved. Applies within a minute.",
  "settings.selfUpdate": "HiveDock self-update",
  "settings.selfUpdateHelp":
    "Release images are cosign-signed via GitHub Actions. HiveDock verifies that signature and pins the exact digest before offering or applying an update to itself.",
  "settings.saved": "Saved.",
  "settings.versionHistory": "Version history",
  "settings.apiToken": "Read-only API token",
  "settings.registries": "Private registries",
  "settings.maintenance": "Maintenance",
  "settings.environment": "Environment",
  "settings.environmentHelp":
    "Configured via environment variables (change requires a container restart).",
  "settings.failedSave": "Failed to save.",

  "settings.interval.off": "Off",
  "settings.interval.15m": "Every 15 minutes",
  "settings.interval.30m": "Every 30 minutes",
  "settings.interval.1h": "Every hour",
  "settings.interval.3h": "Every 3 hours",
  "settings.interval.6h": "Every 6 hours",
  "settings.interval.12h": "Every 12 hours",
  "settings.interval.24h": "Every 24 hours",

  "settings.updateMode.full": "Full",
  "settings.updateMode.fullDesc":
    "Check for new releases, verify their signatures, and allow one-click updates from the sidebar.",
  "settings.updateMode.checkOnly": "Check only",
  "settings.updateMode.checkOnlyDesc":
    "Check and verify, but never apply automatically. Update manually from a shell.",
  "settings.updateMode.off": "Off",
  "settings.updateMode.offDesc": "No version check at all (for air-gapped installs).",

  "settings.git.help":
    "Records every change under your stacks directory to a local git repo: HiveDock's own edits and changes made outside the UI alike. Local only, no remotes, no pushing. Useful for auditing and rollback.",
  "settings.git.toggle": "Commit stack changes to git",
  "settings.git.toggleDesc":
    "A snapshot commit captures any out-of-band change first, then each HiveDock write is its own commit (authored “HiveDock”).",
  "settings.git.notRepo": "Your stacks directory {{dir}} is not a git repository yet.",
  "settings.git.init": "Initialize git repository",
  "settings.git.initializing": "Initializing…",
  "settings.git.initialized":
    "Initialized. Turn on version history to start recording changes.",
  "settings.git.on": "On. Changes are now committed locally.",
  "settings.git.off": "Off.",
  "settings.git.failedInit": "Failed to initialize.",

  "settings.token.help":
    "A bearer token for monitoring tools (uptime-kuma, gatus, scripts). It works only on GET /api/health, /api/stacks, and /api/updates, never mutations or settings. Stored as a hash, shown once.",
  "settings.token.copyNow": "Copy it now, it won't be shown again.",
  "settings.token.copy": "Copy",
  "settings.token.generate": "Generate token",
  "settings.token.regenerate": "Regenerate token",
  "settings.token.revoke": "Revoke",
  "settings.token.active": "A token is active.",
  "settings.token.none": "No token yet.",
  "settings.token.revoked": "Token revoked.",
  "settings.token.failedGen": "Failed to generate.",
  "settings.token.failedRevoke": "Failed to revoke.",

  "settings.reg.help":
    "Credentials for private registries and TLS trust for self-signed ones. Only registries listed here get credentials, everything else stays anonymous with strict TLS. Passwords are stored under DATA_DIR (not encrypted at rest) and never shown again.",
  "settings.reg.hostPh": "registry host (registry.example.com)",
  "settings.reg.userPh": "username (optional)",
  "settings.reg.passPh": "password / token (optional)",
  "settings.reg.caPh": "CA bundle path (optional)",
  "settings.reg.skipTls": "Skip TLS verification (self-signed)",
  "settings.reg.add": "Add / update",
  "settings.reg.anonymous": "anonymous",
  "settings.reg.as": "as {{user}}",
  "settings.reg.passwordSet": "password set",
  "settings.reg.customCa": "custom CA",
  "settings.reg.tlsOff": "TLS off",
  "settings.reg.failedRemove": "Failed to remove.",

  "settings.prune.help":
    "Image updates leave the old, now-untagged image layers behind on disk. Prune removes those dangling images and stale build cache. It never touches tagged images, containers, volumes, or networks, so it is safe to run any time.",
  "settings.prune.button": "Prune dangling images",
  "settings.prune.pruning": "Pruning…",
  "settings.prune.nothing": "Nothing to prune, already clean.",
  "settings.prune.failed": "Prune failed.",
  "settings.prune.removed": "Removed {{n}} dangling images, reclaimed {{size}}.",

  "settings.env.stacksDir": "Stacks directory",
  "settings.env.dataDir": "Data directory",
  "settings.env.publicHost": "Public host",
  "settings.env.auth": "Authentication",
  "settings.env.version": "Version",
  "settings.env.requestHost": "(request host)",

  "theme.hive.blurb": "The original cool-gray console with honeycomb amber.",
  "theme.glossy.blurb": "Frosted-glass panels over a violet gradient.",
  "theme.paper.blurb": "Serif ink on off-white with hairline rules.",
  "theme.fallout.blurb": "Pip-Boy phosphor green on a CRT, scanlines and all.",
  "theme.cyberpunk.blurb": "Pixel-art neon: hot magenta and cyan on midnight.",
  "theme.nord.blurb": "Arctic calm: muted blue-grays with icy frost accents.",

  "dashboard.search": "Search apps…",
  "dashboard.showHidden": "Show hidden ({{n}})",
  "dashboard.active": "active",
  "dashboard.inactive": "inactive",
  "dashboard.exited": "exited",
  "dashboard.customize": "Customize",
  "dashboard.displayName": "Display name",
  "dashboard.iconUrl": "Icon URL or slug",
  "dashboard.iconHelp": "Icon: a full image URL, or a dashboard-icons name. Leave a field empty to go back to automatic.",
  "dashboard.linkUrl": "Link URL",
  "dashboard.linkHelp":
    "Where the tile links. Set this for apps whose port HiveDock can't detect: host-network containers, or one sharing another's network (e.g. behind Gluetun). Empty = automatic.",

  "stacks.new": "New",
  "stacks.deploy": "Deploy",
  "stacks.pull": "Pull",
  "stacks.restart": "Restart",
  "stacks.stop": "Stop",
  "stacks.rename": "Rename",
  "stacks.delete": "Delete",
  "stacks.validate": "Validate",
  "stacks.revert": "Revert",
  "stacks.logs": "Logs",
  "stacks.follow": "Follow",
  "stacks.enlarge": "Enlarge",
  "stacks.managed": "Managed",
  "stacks.external": "External",
  "stacks.runOp": "Run an operation to see its live output here.",
  "stacks.starting": "Starting…",
  "stacks.saveNote": "Save writes the file; it does not redeploy.",

  "updates.checkNow": "Check now",
  "updates.checking": "Checking…",
  "updates.allUpToDate": "Everything up to date",
  "updates.oneAvailable": "1 update available",
  "updates.manyAvailable": "{{n}} updates available",
  "updates.ignore": "Ignore",
  "updates.unignore": "Un-ignore",
  "updates.selectAll": "Select all",
  "updates.updateSelected": "Update selected ({{n}})",
  "updates.updateAll": "Update all ({{n}})",
  "updates.updating": "Updating…",
  "updates.updateRedeploy": "Update & redeploy",
  "updates.pullRedeploy": "Pull & redeploy",
  "updates.sectionAvailable": "Update available",
  "updates.sectionIgnored": "Ignored",
  "updates.sectionUpToDate": "Up to date",
  "updates.sectionOther": "Not checked / other",
  "updates.usedBy": "Used by:",
  "updates.changelog": "Changelog / source",
  "updates.checkedAt": "Checked {{when}}",
  "updates.lastChecked": "Last checked {{ago}}",
  "updates.empty": "No managed stacks with images to check. Create a stack, then Check now.",
  "updates.chip.uptodate": "up to date",
  "updates.chip.notChecked": "not checked",
  "updates.chip.error": "error",
  "updates.chip.unsupported": "unsupported",
  "updates.chip.checked": "checked",
  "updates.chip.envManaged": "env-managed",
  "updates.newDigest": "new digest",
  "time.justNow": "just now",
  "time.mAgo": "{{n}}m ago",
  "time.hAgo": "{{n}}h ago",
  "time.dAgo": "{{n}}d ago",
};

const pl: Dict = {
  "nav.home": "Start",
  "nav.stacks": "Stacks",
  "nav.updates": "Aktualizacje",
  "nav.settings": "Ustawienia",
  "sidebar.backendOk": "Serwer OK",
  "sidebar.backendDown": "Serwer niedostępny",
  "sidebar.signOut": "Wyloguj",

  "common.save": "Zapisz",
  "common.saving": "Zapisywanie…",
  "common.cancel": "Anuluj",
  "common.active": "Aktywny",
  "common.remove": "Usuń",

  "settings.title": "Ustawienia",
  "settings.appearance": "Wygląd",
  "settings.appearanceHelp":
    "Motyw jest zapisywany tylko w tej przeglądarce. Nie jest współdzielony z innymi użytkownikami ani urządzeniami.",
  "settings.language": "Język",
  "settings.languageHelp": "Język interfejsu, zapisywany w tej przeglądarce.",
  "settings.autoUpdate": "Automatyczne sprawdzanie aktualizacji",
  "settings.autoUpdateHelp":
    "Jak często HiveDock sprawdza w tle nowsze obrazy w rejestrach. Zmiany zaczynają obowiązywać w ciągu minuty, bez restartu.",
  "settings.autoUpdateSaved": "Zapisano. Zacznie obowiązywać w ciągu minuty.",
  "settings.selfUpdate": "Samoaktualizacja HiveDock",
  "settings.selfUpdateHelp":
    "Obrazy wydań są podpisane przez cosign w GitHub Actions. HiveDock weryfikuje ten podpis i przypina dokładny digest przed zaproponowaniem lub zastosowaniem własnej aktualizacji.",
  "settings.saved": "Zapisano.",
  "settings.versionHistory": "Historia wersji",
  "settings.apiToken": "Token API tylko do odczytu",
  "settings.registries": "Prywatne rejestry",
  "settings.maintenance": "Konserwacja",
  "settings.environment": "Środowisko",
  "settings.environmentHelp":
    "Konfigurowane przez zmienne środowiskowe (zmiana wymaga restartu kontenera).",
  "settings.failedSave": "Nie udało się zapisać.",

  "settings.interval.off": "Wyłączone",
  "settings.interval.15m": "Co 15 minut",
  "settings.interval.30m": "Co 30 minut",
  "settings.interval.1h": "Co godzinę",
  "settings.interval.3h": "Co 3 godziny",
  "settings.interval.6h": "Co 6 godzin",
  "settings.interval.12h": "Co 12 godzin",
  "settings.interval.24h": "Co 24 godziny",

  "settings.updateMode.full": "Pełna",
  "settings.updateMode.fullDesc":
    "Sprawdzaj nowe wydania, weryfikuj podpisy i zezwalaj na aktualizację jednym kliknięciem z panelu bocznego.",
  "settings.updateMode.checkOnly": "Tylko sprawdzanie",
  "settings.updateMode.checkOnlyDesc":
    "Sprawdzaj i weryfikuj, ale nigdy nie stosuj automatycznie. Aktualizuj ręcznie z powłoki.",
  "settings.updateMode.off": "Wyłączona",
  "settings.updateMode.offDesc":
    "Brak sprawdzania wersji (dla instalacji odciętych od sieci).",

  "settings.git.help":
    "Zapisuje każdą zmianę w katalogu stacków do lokalnego repozytorium git: zarówno edycje HiveDock, jak i zmiany spoza UI. Tylko lokalnie, bez zdalnych repo, bez wysyłania. Przydatne do audytu i cofania zmian.",
  "settings.git.toggle": "Zapisuj zmiany stacków do git",
  "settings.git.toggleDesc":
    "Najpierw commit-migawka rejestruje zmiany spoza UI, potem każdy zapis HiveDock to osobny commit (autor „HiveDock”).",
  "settings.git.notRepo": "Twój katalog stacków {{dir}} nie jest jeszcze repozytorium git.",
  "settings.git.init": "Zainicjuj repozytorium git",
  "settings.git.initializing": "Inicjowanie…",
  "settings.git.initialized":
    "Zainicjowano. Włącz historię wersji, aby zacząć zapisywać zmiany.",
  "settings.git.on": "Włączone. Zmiany są teraz commitowane lokalnie.",
  "settings.git.off": "Wyłączone.",
  "settings.git.failedInit": "Nie udało się zainicjować.",

  "settings.token.help":
    "Token bearer dla narzędzi monitorujących (uptime-kuma, gatus, skrypty). Działa tylko na GET /api/health, /api/stacks i /api/updates, nigdy na zmianach ani ustawieniach. Przechowywany jako hash, pokazywany raz.",
  "settings.token.copyNow": "Skopiuj teraz, nie zostanie pokazany ponownie.",
  "settings.token.copy": "Kopiuj",
  "settings.token.generate": "Wygeneruj token",
  "settings.token.regenerate": "Wygeneruj ponownie",
  "settings.token.revoke": "Unieważnij",
  "settings.token.active": "Token jest aktywny.",
  "settings.token.none": "Brak tokenu.",
  "settings.token.revoked": "Token unieważniony.",
  "settings.token.failedGen": "Nie udało się wygenerować.",
  "settings.token.failedRevoke": "Nie udało się unieważnić.",

  "settings.reg.help":
    "Poświadczenia dla prywatnych rejestrów i zaufanie TLS dla samopodpisanych. Tylko wymienione tu rejestry otrzymują poświadczenia, reszta pozostaje anonimowa ze ścisłym TLS. Hasła są przechowywane w DATA_DIR (bez szyfrowania) i nigdy nie są pokazywane ponownie.",
  "settings.reg.hostPh": "host rejestru (registry.example.com)",
  "settings.reg.userPh": "nazwa użytkownika (opcjonalnie)",
  "settings.reg.passPh": "hasło / token (opcjonalnie)",
  "settings.reg.caPh": "ścieżka pakietu CA (opcjonalnie)",
  "settings.reg.skipTls": "Pomiń weryfikację TLS (samopodpisany)",
  "settings.reg.add": "Dodaj / zaktualizuj",
  "settings.reg.anonymous": "anonimowo",
  "settings.reg.as": "jako {{user}}",
  "settings.reg.passwordSet": "hasło ustawione",
  "settings.reg.customCa": "własny CA",
  "settings.reg.tlsOff": "TLS wyłączone",
  "settings.reg.failedRemove": "Nie udało się usunąć.",

  "settings.prune.help":
    "Aktualizacje obrazów pozostawiają na dysku stare, nieoznaczone warstwy. Czyszczenie usuwa te wiszące obrazy i nieaktualną pamięć podręczną budowania. Nigdy nie narusza oznaczonych obrazów, kontenerów, wolumenów ani sieci, więc jest bezpieczne o każdej porze.",
  "settings.prune.button": "Wyczyść wiszące obrazy",
  "settings.prune.pruning": "Czyszczenie…",
  "settings.prune.nothing": "Nie ma czego czyścić, już czysto.",
  "settings.prune.failed": "Czyszczenie nie powiodło się.",
  "settings.prune.removed": "Usunięto wiszące obrazy: {{n}}, odzyskano {{size}}.",

  "settings.env.stacksDir": "Katalog stacków",
  "settings.env.dataDir": "Katalog danych",
  "settings.env.publicHost": "Host publiczny",
  "settings.env.auth": "Uwierzytelnianie",
  "settings.env.version": "Wersja",
  "settings.env.requestHost": "(host żądania)",

  "theme.hive.blurb": "Oryginalna chłodna szara konsola z bursztynem plastra miodu.",
  "theme.glossy.blurb": "Matowe szklane panele na fioletowym gradiencie.",
  "theme.paper.blurb": "Szeryfowy atrament na złamanej bieli z cienkimi liniami.",
  "theme.fallout.blurb": "Fosforowa zieleń Pip-Boya na CRT, ze skanlinami.",
  "theme.cyberpunk.blurb": "Pikselowy neon: gorąca magenta i cyjan o północy.",
  "theme.nord.blurb": "Arktyczny spokój: stonowane niebieskie szarości z lodowymi akcentami.",

  "dashboard.search": "Szukaj aplikacji…",
  "dashboard.showHidden": "Pokaż ukryte ({{n}})",
  "dashboard.active": "aktywne",
  "dashboard.inactive": "nieaktywne",
  "dashboard.exited": "zakończone",
  "dashboard.customize": "Dostosuj",
  "dashboard.displayName": "Wyświetlana nazwa",
  "dashboard.iconUrl": "URL ikony lub slug",
  "dashboard.iconHelp": "Ikona: pełny URL obrazu lub nazwa z dashboard-icons. Zostaw pole puste, aby wrócić do automatycznej.",
  "dashboard.linkUrl": "URL odnośnika",
  "dashboard.linkHelp":
    "Dokąd prowadzi kafelek. Ustaw dla aplikacji, których portu HiveDock nie wykrywa: kontenerów w sieci hosta lub współdzielących cudzą sieć (np. za Gluetun). Puste = automatycznie.",

  "stacks.new": "Nowy",
  "stacks.deploy": "Wdróż",
  "stacks.pull": "Pobierz",
  "stacks.restart": "Uruchom ponownie",
  "stacks.stop": "Zatrzymaj",
  "stacks.rename": "Zmień nazwę",
  "stacks.delete": "Usuń",
  "stacks.validate": "Sprawdź",
  "stacks.revert": "Przywróć",
  "stacks.logs": "Logi",
  "stacks.follow": "Śledź",
  "stacks.enlarge": "Powiększ",
  "stacks.managed": "Zarządzany",
  "stacks.external": "Zewnętrzny",
  "stacks.runOp": "Uruchom operację, aby zobaczyć tutaj jej wynik na żywo.",
  "stacks.starting": "Uruchamianie…",
  "stacks.saveNote": "Zapis zapisuje plik, nie wdraża ponownie.",

  "updates.checkNow": "Sprawdź teraz",
  "updates.checking": "Sprawdzanie…",
  "updates.allUpToDate": "Wszystko aktualne",
  "updates.oneAvailable": "1 dostępna aktualizacja",
  "updates.manyAvailable": "Dostępne aktualizacje: {{n}}",
  "updates.ignore": "Ignoruj",
  "updates.unignore": "Przywróć",
  "updates.selectAll": "Zaznacz wszystko",
  "updates.updateSelected": "Aktualizuj zaznaczone ({{n}})",
  "updates.updateAll": "Aktualizuj wszystko ({{n}})",
  "updates.updating": "Aktualizowanie…",
  "updates.updateRedeploy": "Aktualizuj i wdróż",
  "updates.pullRedeploy": "Pobierz i wdróż",
  "updates.sectionAvailable": "Dostępna aktualizacja",
  "updates.sectionIgnored": "Ignorowane",
  "updates.sectionUpToDate": "Aktualne",
  "updates.sectionOther": "Niesprawdzone / inne",
  "updates.usedBy": "Używane przez:",
  "updates.changelog": "Dziennik zmian / źródło",
  "updates.checkedAt": "Sprawdzono {{when}}",
  "updates.lastChecked": "Ostatnio sprawdzano {{ago}}",
  "updates.empty":
    "Brak zarządzanych stacków z obrazami do sprawdzenia. Utwórz stack, potem Sprawdź teraz.",
  "updates.chip.uptodate": "aktualne",
  "updates.chip.notChecked": "niesprawdzone",
  "updates.chip.error": "błąd",
  "updates.chip.unsupported": "nieobsługiwane",
  "updates.chip.checked": "sprawdzone",
  "updates.chip.envManaged": "zarządzane env",
  "updates.newDigest": "nowy digest",
  "time.justNow": "przed chwilą",
  "time.mAgo": "{{n}} min temu",
  "time.hAgo": "{{n}} godz. temu",
  "time.dAgo": "{{n}} dni temu",
};

const de: Dict = {
  "nav.home": "Start",
  "nav.stacks": "Stacks",
  "nav.updates": "Updates",
  "nav.settings": "Einstellungen",
  "sidebar.backendOk": "Backend OK",
  "sidebar.backendDown": "Backend nicht erreichbar",
  "sidebar.signOut": "Abmelden",

  "common.save": "Speichern",
  "common.saving": "Speichern…",
  "common.cancel": "Abbrechen",
  "common.active": "Aktiv",
  "common.remove": "Entfernen",

  "settings.title": "Einstellungen",
  "settings.appearance": "Darstellung",
  "settings.appearanceHelp":
    "Das Thema wird nur in diesem Browser gespeichert. Es wird nicht mit anderen Benutzern oder Geräten geteilt.",
  "settings.language": "Sprache",
  "settings.languageHelp": "Die Sprache der Oberfläche, in diesem Browser gespeichert.",
  "settings.autoUpdate": "Automatische Update-Prüfung",
  "settings.autoUpdateHelp":
    "Wie oft HiveDock im Hintergrund die Registries auf neuere Images prüft. Änderungen gelten innerhalb einer Minute, kein Neustart nötig.",
  "settings.autoUpdateSaved": "Gespeichert. Gilt innerhalb einer Minute.",
  "settings.selfUpdate": "HiveDock-Selbstaktualisierung",
  "settings.selfUpdateHelp":
    "Release-Images werden über GitHub Actions mit cosign signiert. HiveDock prüft diese Signatur und pinnt den exakten Digest, bevor es sich selbst aktualisiert.",
  "settings.saved": "Gespeichert.",
  "settings.versionHistory": "Versionsverlauf",
  "settings.apiToken": "Schreibgeschütztes API-Token",
  "settings.registries": "Private Registries",
  "settings.maintenance": "Wartung",
  "settings.environment": "Umgebung",
  "settings.environmentHelp":
    "Über Umgebungsvariablen konfiguriert (Änderung erfordert Container-Neustart).",
  "settings.failedSave": "Speichern fehlgeschlagen.",

  "settings.interval.off": "Aus",
  "settings.interval.15m": "Alle 15 Minuten",
  "settings.interval.30m": "Alle 30 Minuten",
  "settings.interval.1h": "Stündlich",
  "settings.interval.3h": "Alle 3 Stunden",
  "settings.interval.6h": "Alle 6 Stunden",
  "settings.interval.12h": "Alle 12 Stunden",
  "settings.interval.24h": "Alle 24 Stunden",

  "settings.updateMode.full": "Vollständig",
  "settings.updateMode.fullDesc":
    "Auf neue Releases prüfen, Signaturen verifizieren und Ein-Klick-Updates aus der Seitenleiste erlauben.",
  "settings.updateMode.checkOnly": "Nur prüfen",
  "settings.updateMode.checkOnlyDesc":
    "Prüfen und verifizieren, aber nie automatisch anwenden. Manuell über eine Shell aktualisieren.",
  "settings.updateMode.off": "Aus",
  "settings.updateMode.offDesc":
    "Gar keine Versionsprüfung (für Air-Gapped-Installationen).",

  "settings.git.help":
    "Zeichnet jede Änderung im Stacks-Verzeichnis in einem lokalen Git-Repo auf: sowohl HiveDocks eigene Änderungen als auch solche außerhalb der UI. Nur lokal, keine Remotes, kein Push. Nützlich für Audit und Rollback.",
  "settings.git.toggle": "Stack-Änderungen in Git committen",
  "settings.git.toggleDesc":
    "Ein Snapshot-Commit erfasst zuerst externe Änderungen, dann ist jeder HiveDock-Schreibvorgang ein eigener Commit (Autor „HiveDock“).",
  "settings.git.notRepo": "Ihr Stacks-Verzeichnis {{dir}} ist noch kein Git-Repository.",
  "settings.git.init": "Git-Repository initialisieren",
  "settings.git.initializing": "Initialisieren…",
  "settings.git.initialized":
    "Initialisiert. Schalten Sie den Versionsverlauf ein, um Änderungen aufzuzeichnen.",
  "settings.git.on": "Ein. Änderungen werden jetzt lokal committet.",
  "settings.git.off": "Aus.",
  "settings.git.failedInit": "Initialisierung fehlgeschlagen.",

  "settings.token.help":
    "Ein Bearer-Token für Monitoring-Tools (uptime-kuma, gatus, Skripte). Es funktioniert nur bei GET /api/health, /api/stacks und /api/updates, nie bei Änderungen oder Einstellungen. Als Hash gespeichert, einmal angezeigt.",
  "settings.token.copyNow": "Jetzt kopieren, es wird nicht erneut angezeigt.",
  "settings.token.copy": "Kopieren",
  "settings.token.generate": "Token erzeugen",
  "settings.token.regenerate": "Token neu erzeugen",
  "settings.token.revoke": "Widerrufen",
  "settings.token.active": "Ein Token ist aktiv.",
  "settings.token.none": "Noch kein Token.",
  "settings.token.revoked": "Token widerrufen.",
  "settings.token.failedGen": "Erzeugen fehlgeschlagen.",
  "settings.token.failedRevoke": "Widerruf fehlgeschlagen.",

  "settings.reg.help":
    "Anmeldedaten für private Registries und TLS-Vertrauen für selbstsignierte. Nur die hier aufgeführten Registries erhalten Anmeldedaten, alles andere bleibt anonym mit striktem TLS. Passwörter werden unter DATA_DIR gespeichert (nicht verschlüsselt) und nie erneut angezeigt.",
  "settings.reg.hostPh": "Registry-Host (registry.example.com)",
  "settings.reg.userPh": "Benutzername (optional)",
  "settings.reg.passPh": "Passwort / Token (optional)",
  "settings.reg.caPh": "CA-Bundle-Pfad (optional)",
  "settings.reg.skipTls": "TLS-Prüfung überspringen (selbstsigniert)",
  "settings.reg.add": "Hinzufügen / aktualisieren",
  "settings.reg.anonymous": "anonym",
  "settings.reg.as": "als {{user}}",
  "settings.reg.passwordSet": "Passwort gesetzt",
  "settings.reg.customCa": "eigene CA",
  "settings.reg.tlsOff": "TLS aus",
  "settings.reg.failedRemove": "Entfernen fehlgeschlagen.",

  "settings.prune.help":
    "Image-Updates hinterlassen alte, nun unbenannte Image-Layer auf der Festplatte. Prune entfernt diese verwaisten Images und veralteten Build-Cache. Es berührt nie benannte Images, Container, Volumes oder Netzwerke und ist daher jederzeit sicher.",
  "settings.prune.button": "Verwaiste Images entfernen",
  "settings.prune.pruning": "Bereinigen…",
  "settings.prune.nothing": "Nichts zu bereinigen, bereits sauber.",
  "settings.prune.failed": "Bereinigung fehlgeschlagen.",
  "settings.prune.removed": "{{n}} verwaiste Images entfernt, {{size}} freigegeben.",

  "settings.env.stacksDir": "Stacks-Verzeichnis",
  "settings.env.dataDir": "Datenverzeichnis",
  "settings.env.publicHost": "Öffentlicher Host",
  "settings.env.auth": "Authentifizierung",
  "settings.env.version": "Version",
  "settings.env.requestHost": "(Anfrage-Host)",

  "theme.hive.blurb": "Die originale kühlgraue Konsole mit Honigwaben-Amber.",
  "theme.glossy.blurb": "Milchglas-Panels über einem violetten Verlauf.",
  "theme.paper.blurb": "Serifentinte auf gebrochenem Weiß mit feinen Linien.",
  "theme.fallout.blurb": "Pip-Boy-Phosphorgrün auf einem CRT, mit Scanlines.",
  "theme.cyberpunk.blurb": "Pixel-Neon: heißes Magenta und Cyan um Mitternacht.",
  "theme.nord.blurb": "Arktische Ruhe: gedämpfte Blaugrautöne mit eisigen Akzenten.",

  "dashboard.search": "Apps suchen…",
  "dashboard.showHidden": "Ausgeblendete anzeigen ({{n}})",
  "dashboard.active": "aktiv",
  "dashboard.inactive": "inaktiv",
  "dashboard.exited": "beendet",
  "dashboard.customize": "Anpassen",
  "dashboard.displayName": "Anzeigename",
  "dashboard.iconUrl": "Icon-URL oder Slug",
  "dashboard.iconHelp": "Icon: eine vollständige Bild-URL oder ein dashboard-icons-Name. Feld leer lassen für automatisch.",
  "dashboard.linkUrl": "Link-URL",
  "dashboard.linkHelp":
    "Wohin die Kachel verlinkt. Für Apps setzen, deren Port HiveDock nicht erkennt: Host-Netzwerk-Container oder solche, die das Netzwerk eines anderen teilen (z. B. hinter Gluetun). Leer = automatisch.",

  "stacks.new": "Neu",
  "stacks.deploy": "Bereitstellen",
  "stacks.pull": "Pull",
  "stacks.restart": "Neu starten",
  "stacks.stop": "Stoppen",
  "stacks.rename": "Umbenennen",
  "stacks.delete": "Löschen",
  "stacks.validate": "Prüfen",
  "stacks.revert": "Zurücksetzen",
  "stacks.logs": "Logs",
  "stacks.follow": "Folgen",
  "stacks.enlarge": "Vergrößern",
  "stacks.managed": "Verwaltet",
  "stacks.external": "Extern",
  "stacks.runOp": "Führen Sie eine Operation aus, um hier die Live-Ausgabe zu sehen.",
  "stacks.starting": "Starten…",
  "stacks.saveNote": "Speichern schreibt die Datei, es stellt nicht neu bereit.",

  "updates.checkNow": "Jetzt prüfen",
  "updates.checking": "Prüfen…",
  "updates.allUpToDate": "Alles aktuell",
  "updates.oneAvailable": "1 Update verfügbar",
  "updates.manyAvailable": "{{n}} Updates verfügbar",
  "updates.ignore": "Ignorieren",
  "updates.unignore": "Nicht mehr ignorieren",
  "updates.selectAll": "Alle auswählen",
  "updates.updateSelected": "Ausgewählte aktualisieren ({{n}})",
  "updates.updateAll": "Alle aktualisieren ({{n}})",
  "updates.updating": "Aktualisieren…",
  "updates.updateRedeploy": "Aktualisieren & neu bereitstellen",
  "updates.pullRedeploy": "Pull & neu bereitstellen",
  "updates.sectionAvailable": "Update verfügbar",
  "updates.sectionIgnored": "Ignoriert",
  "updates.sectionUpToDate": "Aktuell",
  "updates.sectionOther": "Nicht geprüft / sonstige",
  "updates.usedBy": "Verwendet von:",
  "updates.changelog": "Changelog / Quelle",
  "updates.checkedAt": "Geprüft {{when}}",
  "updates.lastChecked": "Zuletzt geprüft {{ago}}",
  "updates.empty":
    "Keine verwalteten Stacks mit Images zum Prüfen. Erstellen Sie einen Stack, dann Jetzt prüfen.",
  "updates.chip.uptodate": "aktuell",
  "updates.chip.notChecked": "nicht geprüft",
  "updates.chip.error": "Fehler",
  "updates.chip.unsupported": "nicht unterstützt",
  "updates.chip.checked": "geprüft",
  "updates.chip.envManaged": "env-verwaltet",
  "updates.newDigest": "neuer Digest",
  "time.justNow": "gerade eben",
  "time.mAgo": "vor {{n}} Min.",
  "time.hAgo": "vor {{n}} Std.",
  "time.dAgo": "vor {{n}} T.",
};

const es: Dict = {
  "nav.home": "Inicio",
  "nav.stacks": "Stacks",
  "nav.updates": "Actualizaciones",
  "nav.settings": "Ajustes",
  "sidebar.backendOk": "Backend OK",
  "sidebar.backendDown": "Backend no disponible",
  "sidebar.signOut": "Cerrar sesión",

  "common.save": "Guardar",
  "common.saving": "Guardando…",
  "common.cancel": "Cancelar",
  "common.active": "Activo",
  "common.remove": "Quitar",

  "settings.title": "Ajustes",
  "settings.appearance": "Apariencia",
  "settings.appearanceHelp":
    "El tema se guarda solo en este navegador. No se comparte con otros usuarios ni dispositivos.",
  "settings.language": "Idioma",
  "settings.languageHelp": "El idioma de la interfaz, guardado en este navegador.",
  "settings.autoUpdate": "Comprobación automática de actualizaciones",
  "settings.autoUpdateHelp":
    "Con qué frecuencia HiveDock comprueba en segundo plano imágenes más nuevas en los registros. Los cambios se aplican en un minuto, sin reiniciar.",
  "settings.autoUpdateSaved": "Guardado. Se aplica en un minuto.",
  "settings.selfUpdate": "Autoactualización de HiveDock",
  "settings.selfUpdateHelp":
    "Las imágenes de versión se firman con cosign vía GitHub Actions. HiveDock verifica esa firma y fija el digest exacto antes de ofrecer o aplicar una actualización de sí mismo.",
  "settings.saved": "Guardado.",
  "settings.versionHistory": "Historial de versiones",
  "settings.apiToken": "Token de API de solo lectura",
  "settings.registries": "Registros privados",
  "settings.maintenance": "Mantenimiento",
  "settings.environment": "Entorno",
  "settings.environmentHelp":
    "Configurado mediante variables de entorno (cambiar requiere reiniciar el contenedor).",
  "settings.failedSave": "No se pudo guardar.",

  "settings.interval.off": "Desactivado",
  "settings.interval.15m": "Cada 15 minutos",
  "settings.interval.30m": "Cada 30 minutos",
  "settings.interval.1h": "Cada hora",
  "settings.interval.3h": "Cada 3 horas",
  "settings.interval.6h": "Cada 6 horas",
  "settings.interval.12h": "Cada 12 horas",
  "settings.interval.24h": "Cada 24 horas",

  "settings.updateMode.full": "Completo",
  "settings.updateMode.fullDesc":
    "Comprobar nuevas versiones, verificar sus firmas y permitir actualizaciones con un clic desde la barra lateral.",
  "settings.updateMode.checkOnly": "Solo comprobar",
  "settings.updateMode.checkOnlyDesc":
    "Comprobar y verificar, pero nunca aplicar automáticamente. Actualiza manualmente desde una shell.",
  "settings.updateMode.off": "Desactivado",
  "settings.updateMode.offDesc":
    "Sin comprobación de versión (para instalaciones aisladas).",

  "settings.git.help":
    "Registra cada cambio en tu directorio de stacks en un repositorio git local: tanto las ediciones de HiveDock como los cambios fuera de la UI. Solo local, sin remotos, sin push. Útil para auditoría y reversión.",
  "settings.git.toggle": "Confirmar cambios de stacks en git",
  "settings.git.toggleDesc":
    "Un commit de instantánea captura primero cualquier cambio externo, luego cada escritura de HiveDock es su propio commit (autor “HiveDock”).",
  "settings.git.notRepo": "Tu directorio de stacks {{dir}} aún no es un repositorio git.",
  "settings.git.init": "Inicializar repositorio git",
  "settings.git.initializing": "Inicializando…",
  "settings.git.initialized":
    "Inicializado. Activa el historial de versiones para empezar a registrar cambios.",
  "settings.git.on": "Activado. Los cambios ahora se confirman localmente.",
  "settings.git.off": "Desactivado.",
  "settings.git.failedInit": "No se pudo inicializar.",

  "settings.token.help":
    "Un token bearer para herramientas de monitoreo (uptime-kuma, gatus, scripts). Solo funciona en GET /api/health, /api/stacks y /api/updates, nunca en cambios ni ajustes. Se guarda como hash y se muestra una vez.",
  "settings.token.copyNow": "Cópialo ahora, no se mostrará de nuevo.",
  "settings.token.copy": "Copiar",
  "settings.token.generate": "Generar token",
  "settings.token.regenerate": "Regenerar token",
  "settings.token.revoke": "Revocar",
  "settings.token.active": "Hay un token activo.",
  "settings.token.none": "Aún no hay token.",
  "settings.token.revoked": "Token revocado.",
  "settings.token.failedGen": "No se pudo generar.",
  "settings.token.failedRevoke": "No se pudo revocar.",

  "settings.reg.help":
    "Credenciales para registros privados y confianza TLS para los autofirmados. Solo los registros aquí listados reciben credenciales, el resto permanece anónimo con TLS estricto. Las contraseñas se guardan bajo DATA_DIR (sin cifrar) y nunca se muestran de nuevo.",
  "settings.reg.hostPh": "host del registro (registry.example.com)",
  "settings.reg.userPh": "usuario (opcional)",
  "settings.reg.passPh": "contraseña / token (opcional)",
  "settings.reg.caPh": "ruta del paquete CA (opcional)",
  "settings.reg.skipTls": "Omitir verificación TLS (autofirmado)",
  "settings.reg.add": "Añadir / actualizar",
  "settings.reg.anonymous": "anónimo",
  "settings.reg.as": "como {{user}}",
  "settings.reg.passwordSet": "contraseña establecida",
  "settings.reg.customCa": "CA personalizada",
  "settings.reg.tlsOff": "TLS desactivado",
  "settings.reg.failedRemove": "No se pudo quitar.",

  "settings.prune.help":
    "Las actualizaciones de imágenes dejan en disco las capas antiguas ahora sin etiquetar. La limpieza elimina esas imágenes colgantes y la caché de compilación obsoleta. Nunca toca imágenes etiquetadas, contenedores, volúmenes ni redes, así que es seguro en cualquier momento.",
  "settings.prune.button": "Limpiar imágenes colgantes",
  "settings.prune.pruning": "Limpiando…",
  "settings.prune.nothing": "Nada que limpiar, ya está limpio.",
  "settings.prune.failed": "La limpieza falló.",
  "settings.prune.removed": "Eliminadas {{n}} imágenes colgantes, recuperado {{size}}.",

  "settings.env.stacksDir": "Directorio de stacks",
  "settings.env.dataDir": "Directorio de datos",
  "settings.env.publicHost": "Host público",
  "settings.env.auth": "Autenticación",
  "settings.env.version": "Versión",
  "settings.env.requestHost": "(host de la solicitud)",

  "theme.hive.blurb": "La consola gris fría original con ámbar de panal.",
  "theme.glossy.blurb": "Paneles de vidrio esmerilado sobre un degradado violeta.",
  "theme.paper.blurb": "Tinta con serifas sobre blanco roto con líneas finas.",
  "theme.fallout.blurb": "Verde fósforo Pip-Boy en un CRT, con líneas de escaneo.",
  "theme.cyberpunk.blurb": "Neón pixelado: magenta intenso y cian sobre medianoche.",
  "theme.nord.blurb": "Calma ártica: grises azulados apagados con acentos helados.",

  "dashboard.search": "Buscar apps…",
  "dashboard.showHidden": "Mostrar ocultos ({{n}})",
  "dashboard.active": "activos",
  "dashboard.inactive": "inactivos",
  "dashboard.exited": "detenidos",
  "dashboard.customize": "Personalizar",
  "dashboard.displayName": "Nombre visible",
  "dashboard.iconUrl": "URL o slug del icono",
  "dashboard.iconHelp": "Icono: una URL de imagen completa o un nombre de dashboard-icons. Deja el campo vacío para volver a automático.",
  "dashboard.linkUrl": "URL del enlace",
  "dashboard.linkHelp":
    "A dónde enlaza la tarjeta. Ponla para apps cuyo puerto HiveDock no detecta: contenedores en red del host, o que comparten la red de otro (p. ej. tras Gluetun). Vacío = automático.",

  "stacks.new": "Nuevo",
  "stacks.deploy": "Desplegar",
  "stacks.pull": "Pull",
  "stacks.restart": "Reiniciar",
  "stacks.stop": "Detener",
  "stacks.rename": "Renombrar",
  "stacks.delete": "Eliminar",
  "stacks.validate": "Validar",
  "stacks.revert": "Revertir",
  "stacks.logs": "Registros",
  "stacks.follow": "Seguir",
  "stacks.enlarge": "Ampliar",
  "stacks.managed": "Gestionado",
  "stacks.external": "Externo",
  "stacks.runOp": "Ejecuta una operación para ver aquí su salida en vivo.",
  "stacks.starting": "Iniciando…",
  "stacks.saveNote": "Guardar escribe el archivo, no vuelve a desplegar.",

  "updates.checkNow": "Comprobar ahora",
  "updates.checking": "Comprobando…",
  "updates.allUpToDate": "Todo actualizado",
  "updates.oneAvailable": "1 actualización disponible",
  "updates.manyAvailable": "{{n}} actualizaciones disponibles",
  "updates.ignore": "Ignorar",
  "updates.unignore": "No ignorar",
  "updates.selectAll": "Seleccionar todo",
  "updates.updateSelected": "Actualizar seleccionados ({{n}})",
  "updates.updateAll": "Actualizar todo ({{n}})",
  "updates.updating": "Actualizando…",
  "updates.updateRedeploy": "Actualizar y desplegar",
  "updates.pullRedeploy": "Pull y desplegar",
  "updates.sectionAvailable": "Actualización disponible",
  "updates.sectionIgnored": "Ignorados",
  "updates.sectionUpToDate": "Actualizado",
  "updates.sectionOther": "Sin comprobar / otros",
  "updates.usedBy": "Usado por:",
  "updates.changelog": "Registro de cambios / fuente",
  "updates.checkedAt": "Comprobado {{when}}",
  "updates.lastChecked": "Última comprobación {{ago}}",
  "updates.empty":
    "No hay stacks gestionados con imágenes que comprobar. Crea un stack y luego Comprobar ahora.",
  "updates.chip.uptodate": "actualizado",
  "updates.chip.notChecked": "sin comprobar",
  "updates.chip.error": "error",
  "updates.chip.unsupported": "no compatible",
  "updates.chip.checked": "comprobado",
  "updates.chip.envManaged": "gestionado por env",
  "updates.newDigest": "nuevo digest",
  "time.justNow": "ahora mismo",
  "time.mAgo": "hace {{n}} min",
  "time.hAgo": "hace {{n}} h",
  "time.dAgo": "hace {{n}} d",
};

const fr: Dict = {
  "nav.home": "Accueil",
  "nav.stacks": "Stacks",
  "nav.updates": "Mises à jour",
  "nav.settings": "Paramètres",
  "sidebar.backendOk": "Backend OK",
  "sidebar.backendDown": "Backend injoignable",
  "sidebar.signOut": "Déconnexion",

  "common.save": "Enregistrer",
  "common.saving": "Enregistrement…",
  "common.cancel": "Annuler",
  "common.active": "Actif",
  "common.remove": "Retirer",

  "settings.title": "Paramètres",
  "settings.appearance": "Apparence",
  "settings.appearanceHelp":
    "Le thème est enregistré uniquement dans ce navigateur. Il n'est pas partagé avec d'autres utilisateurs ou appareils.",
  "settings.language": "Langue",
  "settings.languageHelp": "La langue de l'interface, enregistrée dans ce navigateur.",
  "settings.autoUpdate": "Vérification automatique des mises à jour",
  "settings.autoUpdateHelp":
    "À quelle fréquence HiveDock vérifie en arrière-plan les images plus récentes dans les registres. Les changements s'appliquent en une minute, sans redémarrage.",
  "settings.autoUpdateSaved": "Enregistré. S'applique en une minute.",
  "settings.selfUpdate": "Mise à jour automatique de HiveDock",
  "settings.selfUpdateHelp":
    "Les images de version sont signées avec cosign via GitHub Actions. HiveDock vérifie cette signature et épingle le digest exact avant de proposer ou d'appliquer une mise à jour de lui-même.",
  "settings.saved": "Enregistré.",
  "settings.versionHistory": "Historique des versions",
  "settings.apiToken": "Jeton d'API en lecture seule",
  "settings.registries": "Registres privés",
  "settings.maintenance": "Maintenance",
  "settings.environment": "Environnement",
  "settings.environmentHelp":
    "Configuré via des variables d'environnement (une modification nécessite un redémarrage du conteneur).",
  "settings.failedSave": "Échec de l'enregistrement.",

  "settings.interval.off": "Désactivé",
  "settings.interval.15m": "Toutes les 15 minutes",
  "settings.interval.30m": "Toutes les 30 minutes",
  "settings.interval.1h": "Toutes les heures",
  "settings.interval.3h": "Toutes les 3 heures",
  "settings.interval.6h": "Toutes les 6 heures",
  "settings.interval.12h": "Toutes les 12 heures",
  "settings.interval.24h": "Toutes les 24 heures",

  "settings.updateMode.full": "Complet",
  "settings.updateMode.fullDesc":
    "Vérifier les nouvelles versions, vérifier leurs signatures et autoriser les mises à jour en un clic depuis la barre latérale.",
  "settings.updateMode.checkOnly": "Vérifier seulement",
  "settings.updateMode.checkOnlyDesc":
    "Vérifier et valider, mais ne jamais appliquer automatiquement. Mettre à jour manuellement depuis un shell.",
  "settings.updateMode.off": "Désactivé",
  "settings.updateMode.offDesc":
    "Aucune vérification de version (pour les installations isolées).",

  "settings.git.help":
    "Enregistre chaque changement du répertoire des stacks dans un dépôt git local : les modifications de HiveDock comme celles faites hors de l'UI. Local uniquement, sans dépôt distant, sans push. Utile pour l'audit et le retour arrière.",
  "settings.git.toggle": "Valider les changements de stacks dans git",
  "settings.git.toggleDesc":
    "Un commit instantané capture d'abord tout changement externe, puis chaque écriture de HiveDock est son propre commit (auteur “HiveDock”).",
  "settings.git.notRepo": "Votre répertoire des stacks {{dir}} n'est pas encore un dépôt git.",
  "settings.git.init": "Initialiser le dépôt git",
  "settings.git.initializing": "Initialisation…",
  "settings.git.initialized":
    "Initialisé. Activez l'historique des versions pour commencer à enregistrer les changements.",
  "settings.git.on": "Activé. Les changements sont maintenant validés localement.",
  "settings.git.off": "Désactivé.",
  "settings.git.failedInit": "Échec de l'initialisation.",

  "settings.token.help":
    "Un jeton bearer pour les outils de supervision (uptime-kuma, gatus, scripts). Il fonctionne uniquement sur GET /api/health, /api/stacks et /api/updates, jamais sur les modifications ou les paramètres. Stocké sous forme de hachage, affiché une fois.",
  "settings.token.copyNow": "Copiez-le maintenant, il ne sera plus affiché.",
  "settings.token.copy": "Copier",
  "settings.token.generate": "Générer un jeton",
  "settings.token.regenerate": "Régénérer le jeton",
  "settings.token.revoke": "Révoquer",
  "settings.token.active": "Un jeton est actif.",
  "settings.token.none": "Pas encore de jeton.",
  "settings.token.revoked": "Jeton révoqué.",
  "settings.token.failedGen": "Échec de la génération.",
  "settings.token.failedRevoke": "Échec de la révocation.",

  "settings.reg.help":
    "Identifiants pour les registres privés et confiance TLS pour les auto-signés. Seuls les registres listés ici reçoivent des identifiants, tout le reste reste anonyme avec TLS strict. Les mots de passe sont stockés sous DATA_DIR (non chiffrés) et ne sont jamais réaffichés.",
  "settings.reg.hostPh": "hôte du registre (registry.example.com)",
  "settings.reg.userPh": "nom d'utilisateur (facultatif)",
  "settings.reg.passPh": "mot de passe / jeton (facultatif)",
  "settings.reg.caPh": "chemin du bundle CA (facultatif)",
  "settings.reg.skipTls": "Ignorer la vérification TLS (auto-signé)",
  "settings.reg.add": "Ajouter / mettre à jour",
  "settings.reg.anonymous": "anonyme",
  "settings.reg.as": "en tant que {{user}}",
  "settings.reg.passwordSet": "mot de passe défini",
  "settings.reg.customCa": "CA personnalisée",
  "settings.reg.tlsOff": "TLS désactivé",
  "settings.reg.failedRemove": "Échec du retrait.",

  "settings.prune.help":
    "Les mises à jour d'images laissent sur le disque les anciennes couches désormais sans étiquette. Le nettoyage supprime ces images orphelines et le cache de build obsolète. Il ne touche jamais aux images étiquetées, conteneurs, volumes ou réseaux, donc c'est sûr à tout moment.",
  "settings.prune.button": "Nettoyer les images orphelines",
  "settings.prune.pruning": "Nettoyage…",
  "settings.prune.nothing": "Rien à nettoyer, déjà propre.",
  "settings.prune.failed": "Le nettoyage a échoué.",
  "settings.prune.removed": "{{n}} images orphelines supprimées, {{size}} récupérés.",

  "settings.env.stacksDir": "Répertoire des stacks",
  "settings.env.dataDir": "Répertoire de données",
  "settings.env.publicHost": "Hôte public",
  "settings.env.auth": "Authentification",
  "settings.env.version": "Version",
  "settings.env.requestHost": "(hôte de la requête)",

  "theme.hive.blurb": "La console gris froid d'origine avec l'ambre en nid d'abeille.",
  "theme.glossy.blurb": "Panneaux en verre dépoli sur un dégradé violet.",
  "theme.paper.blurb": "Encre à empattements sur blanc cassé avec de fines lignes.",
  "theme.fallout.blurb": "Vert phosphore Pip-Boy sur un CRT, avec les lignes de balayage.",
  "theme.cyberpunk.blurb": "Néon pixelisé : magenta vif et cyan sur minuit.",
  "theme.nord.blurb": "Calme arctique : gris-bleu feutrés avec des accents glacés.",

  "dashboard.search": "Rechercher des apps…",
  "dashboard.showHidden": "Afficher les masqués ({{n}})",
  "dashboard.active": "actifs",
  "dashboard.inactive": "inactifs",
  "dashboard.exited": "arrêtés",
  "dashboard.customize": "Personnaliser",
  "dashboard.displayName": "Nom affiché",
  "dashboard.iconUrl": "URL ou slug de l'icône",
  "dashboard.iconHelp": "Icône : une URL d'image complète ou un nom dashboard-icons. Laissez le champ vide pour revenir à automatique.",
  "dashboard.linkUrl": "URL du lien",
  "dashboard.linkHelp":
    "Où pointe la tuile. À définir pour les apps dont HiveDock ne détecte pas le port : conteneurs en réseau hôte, ou partageant le réseau d'un autre (p. ex. derrière Gluetun). Vide = automatique.",

  "stacks.new": "Nouveau",
  "stacks.deploy": "Déployer",
  "stacks.pull": "Pull",
  "stacks.restart": "Redémarrer",
  "stacks.stop": "Arrêter",
  "stacks.rename": "Renommer",
  "stacks.delete": "Supprimer",
  "stacks.validate": "Valider",
  "stacks.revert": "Rétablir",
  "stacks.logs": "Journaux",
  "stacks.follow": "Suivre",
  "stacks.enlarge": "Agrandir",
  "stacks.managed": "Géré",
  "stacks.external": "Externe",
  "stacks.runOp": "Lancez une opération pour voir sa sortie en direct ici.",
  "stacks.starting": "Démarrage…",
  "stacks.saveNote": "Enregistrer écrit le fichier, cela ne redéploie pas.",

  "updates.checkNow": "Vérifier",
  "updates.checking": "Vérification…",
  "updates.allUpToDate": "Tout est à jour",
  "updates.oneAvailable": "1 mise à jour disponible",
  "updates.manyAvailable": "{{n}} mises à jour disponibles",
  "updates.ignore": "Ignorer",
  "updates.unignore": "Ne plus ignorer",
  "updates.selectAll": "Tout sélectionner",
  "updates.updateSelected": "Mettre à jour la sélection ({{n}})",
  "updates.updateAll": "Tout mettre à jour ({{n}})",
  "updates.updating": "Mise à jour…",
  "updates.updateRedeploy": "Mettre à jour et redéployer",
  "updates.pullRedeploy": "Pull et redéployer",
  "updates.sectionAvailable": "Mise à jour disponible",
  "updates.sectionIgnored": "Ignorés",
  "updates.sectionUpToDate": "À jour",
  "updates.sectionOther": "Non vérifié / autres",
  "updates.usedBy": "Utilisé par :",
  "updates.changelog": "Journal des modifications / source",
  "updates.checkedAt": "Vérifié {{when}}",
  "updates.lastChecked": "Vérifié {{ago}}",
  "updates.empty":
    "Aucun stack géré avec des images à vérifier. Créez un stack, puis Vérifier.",
  "updates.chip.uptodate": "à jour",
  "updates.chip.notChecked": "non vérifié",
  "updates.chip.error": "erreur",
  "updates.chip.unsupported": "non pris en charge",
  "updates.chip.checked": "vérifié",
  "updates.chip.envManaged": "géré par env",
  "updates.newDigest": "nouveau digest",
  "time.justNow": "à l'instant",
  "time.mAgo": "il y a {{n}} min",
  "time.hAgo": "il y a {{n}} h",
  "time.dAgo": "il y a {{n}} j",
};

const dicts: Record<Lang, Dict> = { en, pl, de, es, fr };
const STORAGE_KEY = "hivedock-lang";

function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s;
  return s.replace(/\{\{(\w+)\}\}/g, (_, k) =>
    vars[k] !== undefined ? String(vars[k]) : `{{${k}}}`,
  );
}

export type TFunc = (key: string, vars?: Record<string, string | number>) => string;

function translate(lang: Lang, key: string, vars?: Record<string, string | number>) {
  return interpolate(dicts[lang][key] ?? dicts.en[key] ?? key, vars);
}

type Ctx = { lang: Lang; setLang: (l: Lang) => void; t: TFunc };

// Default context resolves English, so components (and tests) work even without
// a provider; the real app wraps everything in I18nProvider (see main.tsx).
const I18nContext = createContext<Ctx>({
  lang: "en",
  setLang: () => {},
  t: (key, vars) => translate("en", key, vars),
});

function detect(): Lang {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && stored in dicts) return stored as Lang;
    const nav = navigator.language.slice(0, 2);
    if (nav in dicts) return nav as Lang;
  } catch {
    /* localStorage / navigator unavailable */
  }
  return "en";
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detect);
  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);
  const setLang = (l: Lang) => {
    setLangState(l);
    try {
      localStorage.setItem(STORAGE_KEY, l);
    } catch {
      /* ignore */
    }
  };
  const t: TFunc = (key, vars) => translate(lang, key, vars);
  return (
    <I18nContext.Provider value={{ lang, setLang, t }}>{children}</I18nContext.Provider>
  );
}

export function useI18n(): Ctx {
  return useContext(I18nContext);
}
