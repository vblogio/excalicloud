# excalidraw-go

Excalidraw auto-hébergé avec un **gestionnaire de fichiers intégré** : arborescence de dossiers, **vignettes des schémas**, création, renommage, **sélection et déplacement multiples** (glisser-déposer ou dialogue « Déplacer vers… »), duplication, suppression (corbeille), recherche, import/export de fichiers `.excalidraw`, sauvegarde automatique côté serveur. Interface web unique, responsive (l'explorateur devient un tiroir sur mobile).

- **Backend** : Go 1.24, bibliothèque standard uniquement (aucune dépendance externe), API REST/JSON, stockage en fichiers `.excalidraw` sur un volume.
- **Frontend** : React 19 + composant officiel [`@excalidraw/excalidraw`](https://www.npmjs.com/package/@excalidraw/excalidraw) 0.18.1 (licence MIT), build Vite **embarqué dans le binaire Go** (`embed`).
- **Image** : multi-stage, finale en `distroless/static:nonroot` (~53 Mo), un seul conteneur, aucun accès Internet requis à l'exécution (polices Excalidraw servies localement).

## Architecture

```
Navigateur ──HTTP──▶ conteneur excalidraw-go (:8080, uid 65532, rootfs read-only)
                       ├─ /            SPA React + Excalidraw (embarquée dans le binaire)
                       ├─ /api/...     API fichiers (Go net/http)
                       └─ /healthz     sonde
                              │
                              ▼
                       volume /data
                         ├─ projets/réseau/sdn.excalidraw   ← fichiers JSON Excalidraw standards
                         ├─ .library.excalidrawlib          ← bibliothèque partagée
                         ├─ .settings.json, .logo.png       ← titre et logo de la barre
                         ├─ .thumbs/projets/réseau/sdn.excalidraw.png ← miniatures (régénérables)
                         └─ .trash/<horodatage>/...         ← éléments supprimés
```

Les schémas restent des fichiers `.excalidraw` standards : ils s'ouvrent tels quels dans excalidraw.com, VS Code (extension Excalidraw) ou Obsidian, et se sauvegardent/versionnent comme n'importe quel répertoire (rsync, restic, git...).

## Démarrage

```bash
cp .env.example .env        # optionnel : port, authentification Basic
docker compose up -d --build
# http://localhost:8080
```

| Variable        | Défaut   | Rôle                                                              |
|-----------------|----------|-------------------------------------------------------------------|
| `LISTEN_ADDR`   | `:8080`  | Adresse d'écoute                                                  |
| `DATA_DIR`      | `/data`  | Racine des schémas                                                |
| `MAX_BODY_MB`   | `50`     | Taille max d'un schéma (images embarquées en base64 comprises)    |
| `AUTH_USER` / `AUTH_PASSWORD` | vide | Active une authentification HTTP Basic si les deux sont définis |
| `UPDATE_CHECK`  | `true`   | Interroge `registry.npmjs.org` pour signaler une nouvelle version d'Excalidraw (`false` : désactivé) |
| `UPDATE_CHECK_INTERVAL` | `12h` | Intervalle entre deux vérifications (durée Go, minimum `1m`) |
| `NPM_REGISTRY`  | `https://registry.npmjs.org` | Registre interrogé (miroir interne possible) |
| `EXCALIDRAW_VERSION` (build) | vide | Version d'Excalidraw imposée à la construction de l'image ; vide = celle de `package-lock.json` |

Bind mount au lieu du volume nommé : `- ./data:/data` puis `chown -R 65532:65532 ./data`.

## Utilisation

- **＋ Schéma / ＋ Dossier** : création dans le dossier courant.
- **Vignettes / liste** : sélecteur à droite de la recherche (choix mémorisé par navigateur).
- **⋯** sur un élément : renommer, déplacer, dupliquer, télécharger, supprimer.
- **Sélection multiple** : case à cocher (au survol, toujours visible sur écran tactile), `Ctrl/Cmd+clic`, `Maj+clic` pour une plage, `Ctrl+A` pour tout. Tant qu'une sélection est en cours, un simple clic/tap ajoute ou retire un élément. La barre de sélection propose *Déplacer…* et la mise à la corbeille ; `Échap` annule, `Suppr` supprime.
- **Déplacer** : glisser-déposer (souris) d'un élément — ou de toute la sélection — sur un dossier, sur `..` ou sur le fil d'Ariane ; ou *Déplacer…* qui ouvre un sélecteur de dossier (navigation, création de dossier), utilisable au doigt. En cas de conflit de nom dans la destination, les autres éléments sont tout de même déplacés et les échecs listés.
- **Importer** : envoie un ou plusieurs `.excalidraw` locaux dans le dossier courant. Le bouton voisin importe un **dossier complet**.
- **Glisser-déposer depuis le poste** : des fichiers ou des dossiers entiers déposés sur l'explorateur sont importés dans le dossier courant (ou dans le dossier survolé). L'arborescence est recréée, seuls les `.excalidraw` sont importés, les fichiers et dossiers cachés (`.git`, `.DS_Store`) sont ignorés et un schéma déjà présent n'est jamais écrasé. Un bilan s'affiche (détail au survol).
- **Sauvegarde** : automatique 1,5 s après la dernière modification, immédiate avec `Ctrl/Cmd+S`. Le statut (Modifié / Enregistrement… / Enregistré) est affiché dans la barre du haut.
- **Lien direct** : l'URL `http://hôte:8080/#/dossier/schema.excalidraw` ouvre le schéma.
- **Bibliothèque** Excalidraw : persistée côté serveur, partagée entre tous les schémas.
- **Largeur de l'explorateur** (bureau) : glisser la poignée entre l'explorateur et le canevas ; flèches ←/→ au clavier quand elle a le focus (Maj : pas plus grand), double-clic pour revenir à 330 px. Mémorisée par navigateur.
- **Style** : l'explorateur et la barre du haut reprennent les couleurs, la police, les rayons et les tailles de boutons d'Excalidraw (jetons `--ex-*` en tête de `styles.css`), en clair comme en sombre.
- **Mise à jour d'Excalidraw** : quand la version publiée sur le canal choisi (voir *Paramètres*) diffère de celle embarquée, une icône apparaît à droite de la barre du haut ; un clic ouvre les paramètres sur la commande de reconstruction (voir *Mettre à jour Excalidraw*).
- **Mode clair/sombre** : bouton ☀/☾ à droite de la barre du haut ; s'applique à l'interface et au canevas Excalidraw, reste synchronisé avec le choix du menu d'Excalidraw. Mémorisé par navigateur.
- **Paramètres** : bouton ⚙ à droite de la barre (ou clic sur le titre). Section *Apparence* : titre et logo. Section *Excalidraw* : canal suivi, *Stable* (`latest`) ou *Préversion* (`next`), versions publiées et commande de reconstruction. Les paramètres sont stockés dans `DATA_DIR/.settings.json`.
- **Titre et logo** : communs à tous les utilisateurs. Le logo (PNG, 5 Mo max) est recadré au centre, réduit à 64×64 px côté serveur et affiché dans un masque rond (34 px dans la barre, net sur écran HiDPI ; taille réglable via `--logo-size` dans `styles.css`).

### Mettre à jour Excalidraw

La version d'Excalidraw est figée au build (`frontend/package-lock.json`). Le serveur lit les dist-tags du paquet sur le registre npm (`/api/update`, toutes les 12 h par défaut) et l'interface compare la version du canal choisi dans les paramètres à celle embarquée :

- `latest` : versions stables (0.18.1 au 2026-10-07) ;
- `next` : préversion publiée à chaque modification de la branche principale d'Excalidraw (ex. `0.18.0-4f68076`). Son numéro est inférieur à la dernière stable alors que le code est plus récent : l'icône compare donc les versions par égalité, ce qui propose aussi le retour de `next` à `latest`.

Changer de canal dans l'interface ne change rien tant que l'image n'est pas reconstruite. Pour reconstruire avec la version indiquée :

```bash
EXCALIDRAW_VERSION=0.18.2 docker compose up -d --build   # ou EXCALIDRAW_VERSION=… dans .env
```

Les versions `0.x` d'Excalidraw peuvent contenir des changements incompatibles : tester l'image avant de la mettre en service. Pour figer durablement la nouvelle version dans le dépôt : `cd frontend && npm install --save-exact @excalidraw/excalidraw@0.18.2`, puis commiter `package.json` et `package-lock.json`. Sans accès Internet, aucune icône n'est affichée.

### Miniatures

Rendues par le navigateur avec `exportToBlob` d'Excalidraw (PNG 320 px), puis stockées dans `DATA_DIR/.thumbs/` :

- à chaque sauvegarde du schéma ouvert ;
- à la demande pour les schémas sans miniature ou modifiés hors de l'application (import, copie de fichiers sur le volume, script) : l'explorateur charge le schéma, génère la vignette (2 en parallèle, seulement pour les éléments visibles) et l'envoie au serveur, une fois pour toutes.

Une miniature plus ancienne que son schéma est ignorée (`404`) puis régénérée. Elles suivent les renommages, déplacements, copies et suppressions ; `.thumbs/` peut être supprimé sans risque. En thème sombre, elles sont inversées en CSS comme le canevas Excalidraw.

### Concurrence

Chaque lecture renvoie un `ETag` (SHA-256 tronqué du contenu) ; chaque sauvegarde l'envoie en `If-Match`. Si le fichier a été modifié ailleurs (autre onglet, autre poste), le serveur répond `412` et l'interface propose d'écraser ou de recharger. Il n'y a pas d'édition collaborative temps réel (voir *Évolutions*).

## API

Tous les chemins sont relatifs à `DATA_DIR`, en notation `/`. Les composants commençant par `.` sont refusés, et les liens symboliques sortant de la racine sont bloqués.

| Méthode | Route | Corps / paramètres | Réponse |
|---|---|---|---|
| GET | `/api/list?path=dir` | | `[Entry]` (dossiers puis schémas) |
| GET | `/api/search?q=texte` | | `[Entry]` (200 max) |
| GET | `/api/file?path=x.excalidraw` | | JSON du schéma + `ETag` |
| PUT | `/api/file?path=x.excalidraw` | JSON ; `If-Match: <etag>` ou `If-None-Match: *` (création) | `Entry` + `ETag` ; `409` existe, `412` conflit |
| POST | `/api/folder` | `{"path":"a/b"}` | `Entry` |
| POST | `/api/move` | `{"from":"a.excalidraw","to":"d/b.excalidraw"}` | `Entry` (renommage / déplacement, fichier ou dossier) |
| POST | `/api/move-many` | `{"paths":["a.excalidraw","dossier"],"dir":"d"}` | `[{"from","to"}\|{"from","error"}]` : déplacement multiple, résultat par élément |
| GET | `/api/thumb?path=x.excalidraw&v=<modTime>` | | PNG ; `404` si absente ou périmée |
| PUT | `/api/thumb?path=x.excalidraw` | PNG (1 Mo max) | `204` |
| POST | `/api/copy` | `{"from":"…","to":"…"}` | `Entry` |
| DELETE | `/api/entry?path=x[&recursive=true]` | | `204` (déplacé dans `.trash/`) |
| GET/PUT | `/api/library` | bibliothèque `excalidrawlib` | |
| GET | `/api/settings` | | `{"title":"...","logo":"<version>","channel":"latest"}` (`logo` vide sans logo) |
| PUT | `/api/settings` | `{"title":"..."}` (80 caractères max, vide = titre par défaut) et/ou `{"channel":"latest"\|"next"}` | réglages |
| GET | `/api/logo?v=<version>` | | PNG 64×64 ; `404` sans logo |
| PUT | `/api/logo` | PNG (5 Mo, 8192×8192 px max) | réglages ; image recadrée et ré-encodée |
| DELETE | `/api/logo` | | réglages |
| GET | `/api/update` | | `{"enabled","tags":{"latest","next"},"latest","checkedAt"}` |
| GET | `/healthz` | | `ok` (hors authentification) |

`Entry` = `{"name","path","isDir","size","modTime"}`.

Exemple d'automatisation (génération de schémas par script) :

```bash
curl -X PUT 'http://localhost:8080/api/file?path=auto/inventaire.excalidraw' \
     -H 'If-None-Match: *' --data-binary @inventaire.excalidraw
```

## Développement

```bash
# Terminal 1 : API
go run ./cmd/server                 # DATA_DIR=./data par défaut
# Terminal 2 : frontend avec rechargement à chaud (proxy /api -> :8080)
cd frontend && npm install && npm run dev

# Tests et build complet hors Docker
go test ./...
cd frontend && npm run build && cd .. && go build -o server ./cmd/server
```

Arborescence :

```
cmd/server/main.go          serveur HTTP, SPA embarquée, auth Basic, healthcheck
internal/storage/           accès disque : sécurité des chemins, écriture atomique, ETag, corbeille
internal/api/               routes REST
web/embed.go                embarque web/dist (produit par le build Vite)
frontend/src/App.tsx        éditeur Excalidraw, sauvegarde auto, conflits, bibliothèque
frontend/src/Explorer.tsx   gestionnaire de fichiers (vignettes/liste, sélection multiple, glisser-déposer)
frontend/src/MoveDialog.tsx sélecteur de dossier « Déplacer vers… »
frontend/src/Thumb.tsx      vignette chargée à l'affichage, générée si absente
frontend/src/thumbs.ts      rendu PNG des miniatures et file de génération
```

## Sécurité

- Conteneur non-root, rootfs en lecture seule, `cap_drop: ALL`, `no-new-privileges`.
- L'authentification Basic intégrée suffit pour un usage local ; en exposition réseau, placer le service derrière un reverse proxy TLS (Traefik, Caddy, nginx) avec SSO (Authelia, oauth2-proxy, Authentik) et laisser `AUTH_*` vides.
- Écritures atomiques (fichier temporaire + `fsync` + `rename`) : pas de schéma tronqué en cas d'arrêt brutal.
- Aucune suppression physique : la purge de `.trash/` est à la charge de l'exploitant (ex. `find /data/.trash -mindepth 1 -maxdepth 1 -mtime +30 -exec rm -rf {} +`).

## Limites et évolutions possibles

- **Collaboration temps réel** : nécessiterait le serveur [excalidraw-room](https://github.com/excalidraw/excalidraw-room) (WebSocket) et la logique de réconciliation (`reconcileElements`) côté client.
- **Historique de versions** : commit git automatique dans `DATA_DIR` à chaque sauvegarde, ou snapshots du volume (ZFS/Btrfs).
- **Multi-utilisateurs avec espaces privés** : identité transmise par le reverse proxy (`X-Forwarded-User`) et racine par utilisateur.
- Les schémas avec beaucoup d'images embarquées sont volumineux (base64 dans le JSON) ; ajuster `MAX_BODY_MB`.
