// Import de fichiers et d'arborescences (glisser-déposer ou sélecteur de dossier).

export interface PickedFile {
  /** Chemin relatif au point d'import, dossiers compris (« Archi/réseau/x.excalidraw »). */
  rel: string;
  file: File;
}

export interface Collected {
  files: PickedFile[];
  /** Dossiers à recréer (y compris vides), parents avant enfants. */
  dirs: string[];
  /** Fichiers ignorés : pas des schémas Excalidraw. */
  ignored: string[];
}

export const isScene = (name: string) => /\.excalidraw$/i.test(name);

// Fichiers et dossiers cachés (.DS_Store, .git…) : ignorés sans être signalés.
const hidden = (name: string) => name.startsWith(".");

/** Le glisser contient-il des fichiers venant du système ? */
export const hasFiles = (dt: DataTransfer) => dt.types.includes("Files");

const readAll = (reader: FileSystemDirectoryReader) =>
  new Promise<FileSystemEntry[]>((resolve, reject) => {
    const out: FileSystemEntry[] = [];
    // readEntries rend les entrées par lots (100 sous Chrome) : boucler jusqu'au lot vide.
    const next = () =>
      reader.readEntries((batch) => {
        if (batch.length === 0) return resolve(out);
        out.push(...batch);
        next();
      }, reject);
    next();
  });

const fileOf = (entry: FileSystemFileEntry) => new Promise<File>((resolve, reject) => entry.file(resolve, reject));

async function walk(entry: FileSystemEntry, prefix: string, acc: Collected) {
  if (hidden(entry.name)) return;
  const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
  if (entry.isDirectory) {
    acc.dirs.push(rel);
    for (const child of await readAll((entry as FileSystemDirectoryEntry).createReader())) {
      await walk(child, rel, acc);
    }
  } else if (isScene(entry.name)) {
    acc.files.push({ rel, file: await fileOf(entry as FileSystemFileEntry) });
  } else {
    acc.ignored.push(rel);
  }
}

/**
 * Lit un dépôt (fichiers et dossiers). Les entrées doivent être capturées pendant
 * l'événement drop : le DataTransfer est vidé dès que le gestionnaire rend la main.
 */
export function collectDrop(dt: DataTransfer): Promise<Collected> {
  const entries = Array.from(dt.items)
    .filter((i) => i.kind === "file")
    .map((i) => i.webkitGetAsEntry?.())
    .filter((e): e is FileSystemEntry => !!e);
  const plain = entries.length ? [] : Array.from(dt.files); // navigateur sans webkitGetAsEntry
  return (async () => {
    const acc: Collected = { files: [], dirs: [], ignored: [] };
    for (const e of entries) await walk(e, "", acc);
    collectInto(plain, acc);
    return acc;
  })();
}

/** Fichiers d'un <input type="file" webkitdirectory> (chemins dans webkitRelativePath). */
export function collectFiles(files: FileList | File[]): Collected {
  const acc: Collected = { files: [], dirs: [], ignored: [] };
  collectInto(Array.from(files), acc);
  return acc;
}

function collectInto(files: File[], acc: Collected) {
  const dirs = new Set(acc.dirs);
  for (const file of files) {
    const rel = file.webkitRelativePath || file.name;
    const parts = rel.split("/");
    if (parts.some(hidden)) continue;
    for (let i = 1; i < parts.length; i++) dirs.add(parts.slice(0, i).join("/"));
    if (isScene(file.name)) acc.files.push({ rel, file });
    else acc.ignored.push(rel);
  }
  acc.dirs = [...dirs].sort((a, b) => a.split("/").length - b.split("/").length);
}
