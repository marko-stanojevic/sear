# WinPE base image for Kompakt ISO builder.
# Requires Docker in Windows Containers mode.
# Downloads and installs the Windows ADK + WinPE add-on (~2 GB on first build; cached afterwards).

FROM mcr.microsoft.com/windows/servercore:ltsc2022

USER ContainerAdministrator

SHELL ["cmd", "/S", "/C"]

# Copy pre-packaged ADK tools from the Docker build context
# (Requires the adk_tools directory to be placed next to this Dockerfile during manual build)
COPY "adk_tools\Windows Preinstallation Environment" "C:\ADK\Assessment and Deployment Kit\Windows Preinstallation Environment"
COPY "adk_tools\Oscdimg" "C:\ADK\Assessment and Deployment Kit\Deployment Tools\amd64\Oscdimg"

# Add ADK tools to PATH so copype.cmd, oscdimg.exe, and Dism.exe are available.
RUN setx /M PATH "%PATH%;C:\ADK\Assessment and Deployment Kit\Windows Preinstallation Environment\copype;C:\ADK\Assessment and Deployment Kit\Deployment Tools\amd64\Oscdimg"

# Required for oscdimg (UEFI boot files).
RUN xcopy /E /I /Y "C:\ADK\Assessment and Deployment Kit\Windows Preinstallation Environment\amd64\Media" C:\winpe_media_base
