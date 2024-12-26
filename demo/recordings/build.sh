#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------
# build.sh
# Executes VHS from the root of the git repository
# Converts tapes/*.tape to mp4/*.mp4, processes scenes, and generates gifs
# ---------------------------------------------------------------------

# Resolve absolute paths for key directories
REPO_ROOT="$(git rev-parse --show-toplevel)"
TAPES_DIR="$REPO_ROOT/demo/recordings/tapes"
SCENES_DIR="$REPO_ROOT/demo/recordings/scenes"
MP4_OUTDIR="$REPO_ROOT/demo/recordings/mp4"
GIF_OUTDIR="$REPO_ROOT/demo/recordings/gif"
AUDIO_FILE="$REPO_ROOT/demo/recordings/background.mp3"

# Handle "clean" argument
if [[ "${1:-}" == "clean" ]]; then
  echo ">> Cleaning up generated files..."
  rm -rf "$MP4_OUTDIR" "$GIF_OUTDIR"
  exit 0
fi

# Ensure output directories exist
echo ">> Ensuring $MP4_OUTDIR and $GIF_OUTDIR exist"
mkdir -p "$MP4_OUTDIR" "$GIF_OUTDIR"

# 1) Convert each tapes/*.tape => mp4/<basename>.mp4
echo ">> Step 1: Converting $TAPES_DIR/*.tape to $MP4_OUTDIR/*.mp4 via VHS"
shopt -s nullglob
TAPEFILES=( "$TAPES_DIR"/*.tape )
if [[ ${#TAPEFILES[@]} -eq 0 ]]; then
  echo "No .tape files found in $TAPES_DIR. Exiting."
  exit 1
fi

for tape in "${TAPEFILES[@]}"; do
  base="$(basename "$tape" .tape)"
  echo "   Processing $tape -> $MP4_OUTDIR/$base.mp4"
  (cd "$REPO_ROOT" && vhs "$tape" --output "$MP4_OUTDIR/$base.mp4")
done

# 2) Process scenes/*.txt
echo ">> Step 2: Building each scene from $SCENES_DIR/*.txt"
SCENE_FILES=( "$SCENES_DIR"/*.txt )
if [[ ${#SCENE_FILES[@]} -eq 0 ]]; then
  echo "No scene text files found in $SCENES_DIR. Skipping scene-building steps."
  exit 0
fi

for scene_file in "${SCENE_FILES[@]}"; do
  scene_name="$(basename "$scene_file" .txt)"

  echo "   Scene: $scene_file => $scene_name"

  # Concatenate scene
  echo "      Concatenating -> $MP4_OUTDIR/$scene_name.mp4"
  ffmpeg -f concat -safe 0 -i "$scene_file" -c copy "$MP4_OUTDIR/$scene_name.mp4"

  # Add audio fade
  echo "      Adding fade audio -> $MP4_OUTDIR/$scene_name-with-audio.mp4"
  DURATION="$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$MP4_OUTDIR/$scene_name.mp4")"
  FADE_START=$(( ${DURATION%.*} - 5 ))
  ffmpeg -i "$MP4_OUTDIR/$scene_name.mp4" -i "$AUDIO_FILE" \
    -filter_complex "[1:a]afade=t=out:st=${FADE_START}:d=5[aout]" \
    -map 0:v -map "[aout]" \
    -c:v copy -shortest -c:a aac "$MP4_OUTDIR/$scene_name-with-audio.mp4"

  # Create GIF
  echo "      Creating GIF -> $GIF_OUTDIR/$scene_name.gif"
  ffmpeg -y -i "$MP4_OUTDIR/$scene_name-with-audio.mp4" \
    -vf palettegen "$GIF_OUTDIR/$scene_name-palette.png"

  ffmpeg -i "$MP4_OUTDIR/$scene_name-with-audio.mp4" \
         -i "$GIF_OUTDIR/$scene_name-palette.png" \
         -lavfi "fps=10 [video]; [video][1:v] paletteuse" \
         -y "$GIF_OUTDIR/$scene_name.gif"

  echo "      Done with scene: $scene_name"
done

echo
echo ">> Done building scenes!"
echo "   Segments: $MP4_OUTDIR/<segment>.mp4"
echo "   Scenes:   $MP4_OUTDIR/<scene>.mp4"
echo "   Audio:    $MP4_OUTDIR/<scene>-with-audio.mp4"
echo "   GIFs:     $GIF_OUTDIR/<scene>.gif"
echo
echo "Use './build.sh clean' to remove them."
