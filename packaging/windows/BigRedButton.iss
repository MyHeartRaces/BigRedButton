#ifndef SourceDir
#define SourceDir ".\dist\windows-amd64"
#endif

#ifndef OutputDir
#define OutputDir ".\dist"
#endif

#ifndef AppVersion
#define AppVersion "0.2.0"
#endif

[Setup]
AppId={{8D22EA2D-0912-4F12-93A9-52FA7D863D0E}
AppName=Big Red Button
AppVersion={#AppVersion}
AppPublisher=MyHeartRaces
AppPublisherURL=https://github.com/MyHeartRaces/BigRedButton
AppSupportURL=https://github.com/MyHeartRaces/BigRedButton/issues
AppUpdatesURL=https://github.com/MyHeartRaces/BigRedButton/releases
DefaultDirName={autopf}\Big Red Button
DefaultGroupName=Big Red Button
DisableProgramGroupPage=yes
OutputDir={#OutputDir}
OutputBaseFilename=BigRedButtonSetup-{#AppVersion}-windows-amd64
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=lowest
WizardStyle=modern
UninstallDisplayName=Big Red Button

[Files]
Source: "{#SourceDir}\big-red-button.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\big-red-button-gui.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Big Red Button"; Filename: "{app}\big-red-button-gui.exe"; WorkingDir: "{app}"
Name: "{autodesktop}\Big Red Button"; Filename: "{app}\big-red-button-gui.exe"; WorkingDir: "{app}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional shortcuts:"; Flags: unchecked

[Run]
Filename: "{app}\big-red-button-gui.exe"; Description: "Launch Big Red Button"; Flags: nowait postinstall skipifsilent
