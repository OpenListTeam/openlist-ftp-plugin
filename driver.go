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

	pool "github.com/jolestar/go-commons-pool/v2"
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

	commmonConnPoll *pool.ObjectPool
	linkConnPoll    *pool.ObjectPool
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

	if conn, err := d._login(ctx); err != nil {
		return err
	} else {
		_ = conn
		conn.Quit()
	}

	factory := func() pool.PooledObjectFactory {
		return pool.NewPooledObjectFactory(
			func(context.Context) (any, error) {
				return d._login(d.ctx)
			}, func(ctx context.Context, object *pool.PooledObject) error {
				openlistwasiplugindriver.Debugln("FTP: PooledObjectFactory destroy")
				ftpConn := object.Object.(*ftp.ServerConn)
				err := ftpConn.Quit()
				return err
			}, func(ctx context.Context, object *pool.PooledObject) bool {
				ftpConn := object.Object.(*ftp.ServerConn)
				keep := ftpConn.NoOp() == nil
				openlistwasiplugindriver.Debugln("FTP: PooledObjectFactory validate keep:", keep)
				return keep
			}, nil, nil)
	}
	if d.commmonConnPoll == nil {
		config := pool.NewDefaultPoolConfig()
		config.MaxTotal = 10             // 最大连接数
		config.MaxIdle = 5               // 最大空闲连接数
		config.MinIdle = 2               // 最小空闲连接数
		config.TestOnBorrow = true       // 在借用时验证连接的有效性
		config.BlockWhenExhausted = true // 当池耗尽时，让新的请求等待，而不是立即失败
		if d.GeneralPoolSize > 0 {
			config.MaxTotal = d.GeneralPoolSize
			config.MaxIdle = max(d.GeneralPoolSize/2, 1)
		}
		d.commmonConnPoll = pool.NewObjectPool(ctx, factory(), config)
	}
	if d.linkConnPoll == nil {
		config := pool.NewDefaultPoolConfig()
		config.MaxTotal = 10             // 最大连接数
		config.MaxIdle = 5               // 最大空闲连接数
		config.MinIdle = 2               // 最小空闲连接数
		config.TestOnBorrow = true       // 在借用时验证连接的有效性
		config.BlockWhenExhausted = true // 当池耗尽时，让新的请求等待，而不是立即失败
		if d.DownloadPoolSize > 0 {
			config.MaxTotal = d.GeneralPoolSize
			config.MaxIdle = max(d.GeneralPoolSize/2, 1)
		}
		d.linkConnPoll = pool.NewObjectPool(ctx, factory(), config)
	}
	return nil
}

func (d *FTP) Drop(ctx context.Context) error {
	if d.commmonConnPoll != nil {
		d.commmonConnPoll.Close(ctx)
	}
	if d.linkConnPoll != nil {
		d.linkConnPoll.Close(ctx)
	}
	d.cancel()
	return nil
}

func (d *FTP) ListFiles(ctx context.Context, dir drivertypes.Object) ([]drivertypes.Object, error) {
	poll, err := d.commmonConnPoll.BorrowObject(ctx)
	if err != nil {
		return nil, err
	}
	defer d.commmonConnPoll.ReturnObject(ctx, poll)

	entries, err := poll.(*ftp.ServerConn).List(encode(dir.Path, d.Encoding))
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
	link := drivertypes.LinkResourceRangeReader()
	return &link, nil, nil
}

func (d *FTP) LinkRange(ctx context.Context, file drivertypes.Object, args drivertypes.LinkArgs, _range drivertypes.RangeSpec, w io.WriteCloser) error {
	defer w.Close()

	poll, err := d.linkConnPoll.BorrowObject(ctx)
	if err != nil {
		return err
	}
	defer d.linkConnPoll.ReturnObject(ctx, poll)

	path := encode(file.Path, d.Encoding)
	resp, err := poll.(*ftp.ServerConn).RetrFrom(path, _range.Offset)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, io.LimitReader(resp, int64(_range.Size)))
	return err
}

func (d *FTP) MakeDir(ctx context.Context, parentDir drivertypes.Object, dirName string) (*drivertypes.Object, error) {
	poll, err := d.commmonConnPoll.BorrowObject(ctx)
	if err != nil {
		return nil, err
	}
	defer d.commmonConnPoll.ReturnObject(ctx, poll)

	err = poll.(*ftp.ServerConn).MakeDir(encode(path.Join(parentDir.Path, dirName), d.Encoding))
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
	poll, err := d.commmonConnPoll.BorrowObject(ctx)
	if err != nil {
		return nil, err
	}
	defer d.commmonConnPoll.ReturnObject(ctx, poll)

	err = poll.(*ftp.ServerConn).Rename(
		encode(srcObj.Path, d.Encoding),
		encode(path.Join(dstDir.Path, srcObj.Name), d.Encoding),
	)
	if err != nil {
		return nil, err
	}
	return &srcObj, nil
}

func (d *FTP) Rename(ctx context.Context, srcObj drivertypes.Object, newName string) (*drivertypes.Object, error) {
	poll, err := d.commmonConnPoll.BorrowObject(ctx)
	if err != nil {
		return nil, err
	}
	defer d.commmonConnPoll.ReturnObject(ctx, poll)

	err = poll.(*ftp.ServerConn).Rename(
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
	poll, err := d.commmonConnPoll.BorrowObject(ctx)
	if err != nil {
		return err
	}
	defer d.commmonConnPoll.ReturnObject(ctx, poll)

	path := encode(obj.Path, d.Encoding)
	if obj.IsFolder {
		return poll.(*ftp.ServerConn).RemoveDirRecur(path)
	} else {
		return poll.(*ftp.ServerConn).Delete(path)
	}
}

func (d *FTP) Put(ctx context.Context, dstDir drivertypes.Object, file adapter.UploadRequest) (*drivertypes.Object, error) {
	poll, err := d.commmonConnPoll.BorrowObject(ctx)
	if err != nil {
		return nil, err
	}
	defer d.commmonConnPoll.ReturnObject(ctx, poll)

	path := path.Join(dstDir.Path, file.Object.Name)
	stream, err := file.Streams()
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	if err := poll.(*ftp.ServerConn).Stor(encode(path, d.Encoding), stream); err != nil {
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
