
TAPES := $(wildcard atmos-*.tape)
ALL_TAPES :=$(shell (while read -r line; do echo $${line}.tape | tr ' ' '-' | sed 's/---/--/g'; done)<commands.txt)
SCREENSHOTS := $(TAPES:.tape=.png)
INSTALL_PATH := ../../website/static/img/screenshots
# We need to store the artifacts somehwere consistently
# We need to run the demo from the root of the project
# Source is relative to CWD

# Set Working Directory (rejected) https://github.com/charmbracelet/vhs/issues/19
# Set CWD (open) https://github.com/charmbracelet/vhs/issues/320
# Hide doesn't hide https://github.com/charmbracelet/vhs/issues/130
# Fix hide in https://github.com/charmbracelet/vhs/pull/304

# Write to /website/static/img/screenshots

all: $(SCREENSHOTS)

install: $(SCREENSHOTS)
	@echo "Installing screenshots to $(INSTALL_PATH)"
	@mv -f $(SCREENSHOTS) $(INSTALL_PATH)

atmos-%.png: atmos-%.tape
	@echo "Screenshotting $< to $@"
	@#vhs $<
	docker run --rm -v $(PWD)/../../examples:/vhs/examples -v $(PWD):/vhs vhs $<
	@rm -f out.gif

clean:
	rm -f *.gif *.mp4 *.png *.jpg *.ascii

realclean: clean
	rm -f *.tape

%.tape: commands.txt
	@while read -r line; do \
		cmd=$$(echo $$line | tr ' ' '-' | sed 's/---/--/g'); \
		if [ "$$cmd.tape" != "$@" ]; then \
			continue; \
		fi; \
		echo "Creating $$cmd.tape"; \
		echo "# Demo of \`$$line\`" > $$cmd.tape; \
		echo "Source sources/style.tape" >> $$cmd.tape; \
		echo "Source sources/atmos-stacks.tape" >> $$cmd.tape; \
		echo "Output $$cmd.gif" >> $$cmd.tape; \
		echo "Type \"$$line\" Enter Sleep 2500ms " >> $$cmd.tape; \
		echo "Screenshot $$cmd.png" >> $$cmd.tape; \
	done < $<

.PHONY: tapes
tapes: $(ALL_TAPES)

# ffmpeg -y -i /var/folders/xd/jg70jgfs1dj7gdn5dl3hlnkr0000gn/T/vhs4267524947/frame-text-00096.png -i /var/folders/xd/jg70jgfs1dj7gdn5dl3hlnkr0000gn/T/vhs4267524947/frame-cursor-00096.png -f lavfi -i color=#171717:s=1400x800 -i /var/folders/xd/jg70jgfs1dj7gdn5dl3hlnkr0000gn/T/vhs4267524947/bar.png -i /var/folders/xd/jg70jgfs1dj7gdn5dl3hlnkr0000gn/T/vhs4267524947/mask.png -filter_complex \012\011\011[0][1]overlay[merged];\012\011\011[merged]scale=1380:750:force_original_aspect_ratio=1[scaled];\012\011\011[scaled]pad=1400:770:(ow-iw)/2:(oh-ih)/2:#121212[padded];\012\011\011[padded]fillborders=left=10:right=10:top=10:bottom=10:mode=fixed:color=#121212[padded]\012\011\011;\012\011\011\011[3]loop=-1[loopbar];\012\011\011\011[loopbar][padded]overlay=0:30[withbar]\012\011\011\011;\012\011\011\011\011[4]loop=-1[loopmask];\012\011\011\011\011[withbar][loopmask]alphamerge[rounded]\012\011\011\011\011;\012\011\011\011[2]scale=1400:800[bg];\012\011\011\011[bg][rounded]overlay=(W-w)/2:(H-h)/2:shortest=1[withbg]\012\011\011\011 -map [withbg] atmos-completion--help.png
# ffmpeg -y -i /var/folders/xd/jg70jgfs1dj7gdn5dl3hlnkr0000gn/T/vhs4267524947/frame-text-00096.png -i /var/folders/xd/jg70jgfs1dj7gdn5dl3hlnkr0000gn/T/vhs4267524947/frame-cursor-00096.png -f lavfi -i color=#171717:s=1400x800 -i /var/folders/xd/jg70jgfs1dj7gdn5dl3hlnkr0000gn/T/vhs4267524947/bar.png -i /var/folders/xd/jg70jgfs1dj7gdn5dl3hlnkr0000gn/T/vhs4267524947/mask.png -filter_complex \012\011\011[0][1]overlay[merged];\012\011\011[merged]scale=1380:750:force_original_aspect_ratio=1[scaled];\012\011\011[scaled]pad=1400:770:(ow-iw)/2:(oh-ih)/2:#121212[padded];\012\011\011[padded]fillborders=left=10:right=10:top=10:bottom=10:mode=fixed:color=#121212[padded]\012\011\011;\012\011\011\011[3]loop=-1[loopbar];\012\011\011\011[loopbar][padded]overlay=0:30[withbar]\012\011\011\011;\012\011\011\011\011[4]loop=-1[loopmask];\012\011\011\011\011[withbar][loopmask]alphamerge[rounded]\012\011\011\011\011;\012\011\011\011[2]scale=1400:800[bg];\012\011\011\011[bg][rounded]overlay=(W-w)/2:(H-h)/2:shortest=1[withbg]\012\011\011\011 -map [withbg] atmos-completion--help.png
