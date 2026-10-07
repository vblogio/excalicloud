// Package web embarque le frontend compilé (frontend/ -> web/dist) dans le binaire.
package web

import "embed"

// Dist contient le build Vite. En développement, dist/ peut ne contenir que
// .gitkeep : le serveur affiche alors un message invitant à lancer le build.
//
//go:embed all:dist
var Dist embed.FS
