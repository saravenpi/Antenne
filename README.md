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

---

Un projet [Facile Studio](https://github.com/saravenpi).
