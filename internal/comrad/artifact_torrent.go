package comrad

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func ensureArtifactTorrentMetadata(artifact Artifact, artifactDir string) (Artifact, error) {
	if artifact.Torrent != nil && artifact.Torrent.InfoHash != "" && len(artifact.Torrent.MetaInfoBytes) > 0 {
		return artifact, nil
	}
	mi, info, err := buildArtifactMetaInfo(artifact)
	if err != nil {
		return Artifact{}, err
	}
	var body bytes.Buffer
	if err := mi.Write(&body); err != nil {
		return Artifact{}, err
	}
	path, err := writeArtifactMetaInfoFile(artifactDir, artifact.ID, body.Bytes())
	if err != nil {
		return Artifact{}, err
	}
	artifact.Torrent = &ArtifactTorrent{
		InfoHash:      "sha1:" + mi.HashInfoBytes().HexString(),
		MagnetURI:     mi.Magnet(nil, &info).String(),
		PieceLength:   info.PieceLength,
		MetaInfoPath:  path,
		MetaInfoBytes: body.Bytes(),
	}
	return artifact, nil
}

func buildArtifactMetaInfo(artifact Artifact) (*metainfo.MetaInfo, metainfo.Info, error) {
	var info metainfo.Info
	if err := info.BuildFromFilePath(artifact.Path); err != nil {
		return nil, metainfo.Info{}, err
	}
	info.Name = safeArtifactFileName(artifact.ID)
	info.NameUtf8 = info.Name
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		return nil, metainfo.Info{}, err
	}
	return &metainfo.MetaInfo{InfoBytes: infoBytes}, info, nil
}

func writeArtifactMetaInfoFile(artifactDir, artifactID string, body []byte) (string, error) {
	root := strings.TrimSpace(artifactDir)
	if root == "" {
		root = "."
	}
	path := filepath.Join(root, "torrents", safeArtifactFileName(artifactID)+".torrent")
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
