package main

import (
	"context"
	"io"
	"path"
	"time"

	openlistwasiplugindriver "github.com/OpenListTeam/openlist-wasi-plugin-driver"
	"github.com/OpenListTeam/openlist-wasi-plugin-driver/adapter"
	drivertypes "github.com/OpenListTeam/openlist-wasi-plugin-driver/binding/openlist/plugin-driver/types"
	"github.com/jlaffaye/ftp"
)

var _ openlistwasiplugindriver.Driver = (*FTP)(nil)
var _ openlistwasiplugindriver.Reader = (*FTP)(nil)

var _ openlistwasiplugindriver.StreamReader = (*FTP)(nil)
var _ openlistwasiplugindriver.Mkdir = (*FTP)(nil)
var _ openlistwasiplugindriver.Move = (*FTP)(nil)
var _ openlistwasiplugindriver.Rename = (*FTP)(nil)
var _ openlistwasiplugindriver.Remove = (*FTP)(nil)
var _ openlistwasiplugindriver.Copy = (*FTP)(nil)
var _ openlistwasiplugindriver.Put = (*FTP)(nil)

type FTP struct {
	openlistwasiplugindriver.DriverHandle
	Addition

	ctx    context.Context
	cancel context.CancelFunc

	conn *ftp.ServerConn
}

func (*FTP) GetProperties() drivertypes.DriverProps {
	return config
}

func (*FTP) GetFormMeta() []drivertypes.FormField {
	return []drivertypes.FormField{
		{
			Name:  "root_folder_path",
			Label: "RootFolderPath",
			Kind:  drivertypes.FieldKindStringKind("/"),
		},
		{
			Name:     "address",
			Label:    "Address",
			Kind:     drivertypes.FieldKindStringKind(""),
			Required: true,
			Help:     "Service address (e.g., host name, IP:port), required for connecting to the target service",
		},
		{
			Name:     "encoding",
			Label:    "Encoding",
			Kind:     drivertypes.FieldKindStringKind("UTF-8"),
			Required: true,
			Help:     "Data encoding format (e.g., UTF-8, GBK, GB2312, GB18030), required for correct data parsing",
		},
		{
			Name:     "username",
			Label:    "Username",
			Kind:     drivertypes.FieldKindStringKind(""),
			Required: true,
		},
		{
			Name:     "password",
			Label:    "Password",
			Kind:     drivertypes.FieldKindPasswordKind(""),
			Required: true,
		},
		{
			Name:  "general_pool_size",
			Label: "GeneralPoolSize",
			Kind:  drivertypes.FieldKindNumberKind(10),
		},
		{
			Name:  "download_pool_size",
			Label: "DownloadPoolSize",
			Kind:  drivertypes.FieldKindNumberKind(5),
		},
	}
}

func (d *FTP) Init(ctx context.Context) error {
	var err error
	if err = d.LoadConfig(&d.Addition); err != nil {
		return err
	}

	d.Address, err = ProcessFTPAddress(d.Address)
	if err != nil {
		return err
	}
	d.ctx, d.cancel = context.WithCancel(context.Background())
	return d.login()
}

func (d *FTP) Drop(ctx context.Context) error {
	d.conn.Quit()
	d.cancel()
	return nil
}

func (d *FTP) ListFiles(ctx context.Context, dir drivertypes.Object) ([]drivertypes.Object, error) {
	if err := d.login(); err != nil {
		return nil, err
	}

	entries, err := d.conn.List(encode(dir.Path, d.Encoding))
	if err != nil {
		return nil, err
	}
	res := make([]drivertypes.Object, 0, len(entries))
	for _, entry := range entries {
		if entry.Name == "." || entry.Name == ".." {
			continue
		}
		f := drivertypes.Object{
			Name:     decode(entry.Name, d.Encoding),
			Size:     int64(entry.Size),
			Modified: drivertypes.Duration(entry.Time.UnixNano()),
			IsFolder: entry.Type == ftp.EntryTypeFolder,
		}
		res = append(res, f)
	}
	return res, nil
}

func (d *FTP) LinkFile(ctx context.Context, file drivertypes.Object, args drivertypes.LinkArgs) (*drivertypes.LinkResource, *drivertypes.Object, error) {
	if err := d.login(); err != nil {
		return nil, nil, err
	}

	link := drivertypes.LinkResourceRangeReader()
	return &link, nil, nil
}

func (d *FTP) LinkRange(ctx context.Context, file drivertypes.Object, args drivertypes.LinkArgs, _range drivertypes.RangeSpec, w io.WriteCloser) error {
	defer w.Close()
	if err := d.login(); err != nil {
		return err
	}

	path := encode(file.Path, d.Encoding)
	resp, err := d.conn.RetrFrom(path, _range.Offset)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, io.LimitReader(resp, int64(_range.Size)))
	return err
}

func (d *FTP) MakeDir(ctx context.Context, parentDir drivertypes.Object, dirName string) (*drivertypes.Object, error) {
	if err := d.login(); err != nil {
		return nil, err
	}

	err := d.conn.MakeDir(encode(path.Join(parentDir.Path, dirName), d.Encoding))
	if err != nil {
		return nil, err
	}
	return &drivertypes.Object{
		Name:     dirName,
		Modified: drivertypes.Duration(time.Now().UnixNano()),
		IsFolder: true,
	}, nil
}

func (d *FTP) Move(ctx context.Context, srcObj, dstDir drivertypes.Object) (*drivertypes.Object, error) {
	if err := d.login(); err != nil {
		return nil, err
	}
	err := d.conn.Rename(
		encode(srcObj.Path, d.Encoding),
		encode(path.Join(dstDir.Path, srcObj.Name), d.Encoding),
	)
	if err != nil {
		return nil, err
	}
	return &srcObj, nil
}

func (d *FTP) Rename(ctx context.Context, srcObj drivertypes.Object, newName string) (*drivertypes.Object, error) {
	if err := d.login(); err != nil {
		return nil, err
	}
	err := d.conn.Rename(
		encode(srcObj.Path, d.Encoding),
		encode(path.Join(path.Dir(srcObj.Path), newName), d.Encoding),
	)
	if err != nil {
		return nil, err
	}
	srcObj.Name = newName
	return &srcObj, nil
}

func (d *FTP) Copy(ctx context.Context, srcObj, dstDir drivertypes.Object) (*drivertypes.Object, error) {
	return nil, adapter.ErrNotSupport
}

func (d *FTP) Remove(ctx context.Context, obj drivertypes.Object) error {
	if err := d.login(); err != nil {
		return err
	}
	path := encode(obj.Path, d.Encoding)
	if obj.IsFolder {
		return d.conn.RemoveDirRecur(path)
	} else {
		return d.conn.Delete(path)
	}
}

func (d *FTP) Put(ctx context.Context, dstDir drivertypes.Object, file adapter.UploadRequest) (*drivertypes.Object, error) {
	if err := d.login(); err != nil {
		return nil, err
	}

	path := path.Join(dstDir.Path, file.Object.Name)
	stream, err := file.Streams()
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	if err := d.conn.Stor(encode(path, d.Encoding), stream); err != nil {
		return nil, err
	}

	return &drivertypes.Object{
		Path:     path,
		Name:     file.Object.Name,
		Size:     file.Object.Size,
		Created:  file.Object.Created,
		Modified: file.Object.Created,
	}, nil
}
