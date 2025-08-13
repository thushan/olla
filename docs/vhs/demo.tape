###
# VHS Tape for recording olla
# https://github.com/charmbracelet/vhs
###
# Recorded at 1400x1200
# Resized to 700x600 @ 100% Lossless Compression
###

Output docs/artefacts/olla-v1.0.x-demo.gif

# Setup
Set FontSize 22
Set Width 1400
Set Height 1200
Set WindowBar Colorful
Set Theme "JetBrains Darcula"

# intro
Type "# Let's start Olla with a custom configuration"
Enter
Type "# 2x Ollama and 1x LMStudios instances on 3 machines"
Enter
Type "# some are healthy, some are not, let's bring one up"
Enter
Sleep 1s
Type "# then let's run a query for TinyLlama"
Enter
Enter
Sleep 2s
Type "bin/olla -c config/config.local.yaml"
Sleep 1s
Enter
Sleep 50s
