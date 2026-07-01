# Antenne 📡

Déploie une web radio en quelques minutes.

**Antenne** lance une radio en ligne à partir d'un seul binaire :

- 🎙️ **Prise d'antenne en direct** — l'admin prend le micro depuis le navigateur et passe en live par-dessus la musique.
- 🎵 **Diffusion 24/7** — une playlist de fichiers uploadés tourne en boucle en continu, avec crossfade.
- 🚀 **Déploiement rapide** — un binaire Go (+ `ffmpeg`) et un front SvelteKit. Pas d'Icecast ni de Liquidsoap.

## Stack

Monorepo géré avec [`mise`](https://mise.jdx.dev/) :

- **`apps/api`** — Go 1.24, Chi, GORM + Postgres, JWT. Contient **le moteur audio** (playlist, mixage, live, segmentation HLS) et l'API de contrôle. Utilise `ffmpeg` en sous-process pour le décodage/encodage.
- **`apps/client`** — SvelteKit, Svelte 5 (runes), Tailwind v4, **shadcn-svelte**. Design minimaliste noir & blanc, police **Goga**. Player public + console d'admin.

Voir [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) pour le détail du pipeline audio.

## Démarrage rapide

```sh
mise trust && mise install     # toolchains (Go, Node, Postgres, ffmpeg)
mise run bootstrap             # dépendances Go + npm
mise run db-up                 # Postgres local sur 127.0.0.1:5433
cp apps/api/.env.example apps/api/.env
mise run dev                   # API (:4000) + client (:5173)
```

Le player public est sur `http://localhost:5173`, la console admin sur `http://localhost:5173/admin`.

## Concept

Une radio, c'est deux flux : de la musique qui tourne tout le temps, et une voix qui prend l'antenne quand elle le veut. Antenne fait tourner une playlist en boucle et laisse l'admin basculer en direct d'un clic ; les auditeurs écoutent un seul flux continu.

## Intégrer Antenne ailleurs

Antenne expose sa diffusion selon les conventions webradio (ICY/Icecast), donc n'importe quel lecteur ou annuaire peut s'y brancher.

| Ressource | URL | Content-Type |
|---|---|---|
| Flux HLS (navigateur) | `/stream/live.m3u8` | `application/vnd.apple.mpegurl` |
| Flux MP3 continu (VLC, TuneIn, autoradios…) | `/stream.mp3` | `audio/mpeg` |
| Playlist PLS | `/stream.pls` | `audio/x-scpls` |
| Playlist M3U | `/stream.m3u` | `audio/x-mpegurl` |
| Now-playing (JSON) | `/api/now-playing` | `application/json` |

**Écouter :** ouvre `/stream.pls` (ou `/stream.m3u`) dans VLC / un autoradio, ou pointe n'importe quel lecteur sur `/stream.mp3`.

**Intégrer dans une page web :**

```html
<audio controls crossorigin="anonymous" src="https://ta-radio.example.com/stream.mp3"></audio>
```

(`crossorigin` n'est utile que pour un visualiseur Web Audio ; la lecture simple marche sans.)

**Titre en cours :** le flux MP3 injecte les métadonnées ICY `StreamTitle` (envoie l'en-tête `Icy-MetaData: 1`) ; sinon poll `/api/now-playing`. Le flux MP3 est self-healing (ffmpeg est relancé automatiquement) et gère burst-on-connect, clients lents et `TCP_NODELAY`.

**Référencer dans un annuaire** (ex. [radio-browser.info](https://www.radio-browser.info/)) : soumets l'URL publique `/stream.mp3` avec `STATION_PUBLIC=true` (le stream envoie alors `icy-pub: 1`, plus `icy-name`, `icy-genre`, `icy-br`, `icy-url`). Le codec et le bitrate sont auto-détectés.

Réglages dans `apps/api/.env` : `MP3_STREAM_ENABLED`, `MP3_BITRATE_K`, `ICY_METAINT`, `STATION_NAME`, `STATION_GENRE`, `STATION_URL`, `STATION_DESCRIPTION`, `STATION_PUBLIC`.

---

Un projet [Facile Studio](https://github.com/saravenpi).
