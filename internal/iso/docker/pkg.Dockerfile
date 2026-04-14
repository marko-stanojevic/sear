FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

# squashfs-tools 4.6.x (bookworm) supports "mksquashfs - out.squashfs -tar"
# which reads a tar stream from stdin — used for exporting Docker filesystems.
# grub-mkrescue produces hybrid BIOS+UEFI ISOs; mtools is required for EFI.
RUN apt-get update && apt-get install -y --no-install-recommends \
        squashfs-tools \
        xorriso \
        grub-pc-bin \
        grub-efi-amd64-bin \
        grub-common \
        mtools \
    && rm -rf /var/lib/apt/lists/*
