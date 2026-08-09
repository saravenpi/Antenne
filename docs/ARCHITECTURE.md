# Architecture — Poste

Poste est un **binaire Go unique** qui fait tourner une web radio mono-station.
Aucun Icecast ni Liquidsoap : le Go orchestre tout et n'utilise `ffmpeg` que comme
sous-process de **décodage / encodage** (le seul binaire externe requis).

## Vue d'ensemble

```
                    ┌───────────────────────── apps/api (Go) ─────────────────────────┐
 Fichiers uploadés  │                                                                  │
   (disque local)   │   Playlist ──► ffmpeg (decode) ──► PCM ─┐                        │
                    │                                          │                       │
                    │                              ┌──────► Mixer (Go) ──► PCM ──► ffmpeg
 Micro admin        │   WS + Opus ──► decode ──► PCM┘   (switch/duck)            (encode)
 (navigateur)   ────┼──►                                                            │  │
                    │                                                               ▼  │
                    │                                              segments HLS (.ts + .m3u8)
                    │                                                               │  │
                    └───────────────────────────────────────────────────────────┼──┘
                                                                                    │
 Auditeurs  ◄────────────────  GET /stream/live.m3u8  (hls.js)  ◄────────────────────┘
```

## Le pipeline audio

Format PCM interne pivot : **s16le, 48 kHz, stéréo** (`internal/audio/pcm.go`).
Tout est ramené à ce format, mixé, puis ré-encodé une seule fois.

1. **Décodeur playlist** (`playlist.go`)
   Pour la piste courante, un `ffmpeg -i <fichier> -f s16le -ar 48000 -ac 2 -` écrit du
   PCM brut sur un pipe. Le Go lit ce PCM par *frames* de 20 ms. En fin de piste, la
   playlist avance (ordre ou shuffle) et un nouveau décodeur démarre. Le **crossfade**
   se fait en chevauchant les frames de deux décodeurs sur ~2 s.

2. **Source live** (`live.go`)
   L'admin ouvre `WS /api/live/ingest` depuis le navigateur (`MediaRecorder`/WebAudio →
   Opus). Le Go décode en PCM (même format pivot) et pousse les frames dans le mixer.

3. **Mixer / horloge** (`engine.go`)
   Une horloge à 20 ms tire une frame par tick. Tant que le live est inactif, la frame
   vient de la playlist. Quand l'admin « prend l'antenne », le mixer bascule : la
   playlist est *duckée* (baissée) ou remplacée, le live passe au premier plan. Le
   basculement se fait sur un fondu court pour éviter les clics.

4. **Encodeur HLS** (`hls.go`)
   Le PCM mixé est poussé dans `ffmpeg -f s16le ... -f hls -hls_time 4
   -hls_list_size 6 -hls_flags delete_segments live.m3u8`. Les segments `.ts` et le
   `.m3u8` sont écrits dans un répertoire servi en statique.

## Diffusion aux auditeurs

- `GET /stream/live.m3u8` + segments `.ts` → HLS servi en statique.
- Le client utilise **hls.js** (fallback lecture native Safari).
- Latence de type radio : quelques secondes (taille de segment × list size).

## Plan de contrôle (API)

- `POST /api/auth/login` — connexion admin (JWT).
- `GET/POST/DELETE /api/tracks` — upload et gestion des fichiers audio.
- `GET/PUT /api/playlist` — ordre de la playlist, shuffle on/off.
- `GET /api/now-playing` — piste en cours + état live + nb d'auditeurs.
- `WS /api/live/ingest` — flux micro de l'admin (auth requise).
- `POST /api/live/stop` — rendre l'antenne à la playlist.

## Données (Postgres via GORM)

- `admins` — compte(s) admin (bcrypt).
- `tracks` — métadonnées des fichiers (titre, artiste, durée, chemin, position).
- `settings` — état mono-station (nom de la radio, shuffle, crossfade…).

## Déploiement

Un binaire + `ffmpeg` + Postgres + le build statique du client. `docker-compose.yml`
package le tout ; les fichiers audio et les segments HLS vivent sur un volume.
