package main

import (
	"fmt"
	"github.com/schollz/progressbar/v3"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
)

type Downloader struct {
	concurrency int
}

func NewDownloader(concurrency int) *Downloader{
	return &Downloader{concurrency:concurrency}
}

func (d *Downloader) Download(strURL, filename string) error{
	if filename== ""{
		filename = path.Base(strURL)
	}
	
	resp,err :=http.Head(strURL)
	if err != nil{
		return err
	}
	
	if resp.StatusCode ==http.StatusOK && resp.Header.Get("Accept-Ranges")=="bytes"{
		return d.multiDownload(strURL,filename,int(resp.ContentLength))
	}
	return d.singleDownload(strURL,filename)
}

func (d *Downloader) multiDownload(strURL, filename string, contentLen int) error{
	partSize :=contentLen /d.concurrency

	partDir :=d.getPartDir(filename)
	os.Mkdir(partDir,0777)
	defer os.RemoveAll(partDir)

	var wg sync.WaitGroup
	wg.Add(d.concurrency)

	rangeStart:=0

	for i:=0;i<d.concurrency;i++{
		go func(i,rangeStart int) {
			defer wg.Done()

			rangeEnd := rangeStart +partSize

			if i==d.concurrency-1{
				rangeEnd = contentLen
			}
			d.downloadPartial(strURL,filename,rangeStart,rangeEnd,i)
		}(i,rangeStart)
		rangeStart +=partSize+1
	}
	wg.Wait()

	d.merge(filename)

	return nil
}

func (d *Downloader) singleDownload(strURL string, filename string) error {
	req,err:=http.NewRequest("GET",strURL,nil)
	if err!=nil{
		log.Fatal(err)
	}
	resp,err:=http.DefaultClient.Do(req)
	if err!=nil{
		log.Fatal(err)
	}
	defer resp.Body.Close()

	flags :=os.O_CREATE | os.O_WRONLY
	partFile,err :=os.OpenFile(filename,flags,0666)
	if err!=nil{
		log.Fatal(err)
	}
	defer partFile.Close()


	buf :=make([]byte,32*1024)
	_, err = io.CopyBuffer(partFile, resp.Body, buf)
	if err!=nil{
		if err==io.EOF{
			return nil
		}
		log.Fatal(err)
	}

	return nil
}

func (d *Downloader) downloadPartial(strURL,filename string,rangeStart,rangeEnd,i int) {
	if rangeStart>=rangeEnd{
		return
	}

	req,err:=http.NewRequest("GET",strURL,nil)
	if err!=nil{
		log.Fatal(err)
	}
	req.Header.Set("Range",fmt.Sprintf("bytes=%d-%d",rangeStart,rangeEnd))
	resp,err:=http.DefaultClient.Do(req)
	if err!=nil{
		log.Fatal(err)
	}
	defer resp.Body.Close()

	flags :=os.O_CREATE | os.O_WRONLY
	partFile,err :=os.OpenFile(d.getPartFilename(filename,i),flags,0666)
	if err!=nil{
		log.Fatal(err)
	}
	defer partFile.Close()

	bar :=progressbar.Default(resp.ContentLength,"下载")

	buf :=make([]byte,32*1024)
	_, err = io.CopyBuffer(io.MultiWriter(partFile,bar), resp.Body, buf)
	if err!=nil{
		if err==io.EOF{
			return
		}
		log.Fatal(err)
	}

}

func (d *Downloader) getPartFilename(filename string, partNum int) string {
	partDir:=d.getPartDir(filename)
	return fmt.Sprintf("%s/%s-%d",partDir,filename,partNum)
}

func (d *Downloader) getPartDir(filename string) string {
	return strings.SplitN(filename,".",2)[0]
}

func (d *Downloader) merge(filename string) error{
	destFile,err:=os.OpenFile(filename,os.O_WRONLY|os.O_CREATE,0666)
	if err!=nil{
		return err
	}
	defer destFile.Close()

	bar :=progressbar.Default(int64(d.concurrency),"合并")

	for i:=0;i<d.concurrency;i++{
		partFileName :=d.getPartFilename(filename,i)
		partFile,err:=os.Open(partFileName)
		if err!=nil{
			return err
		}
		io.Copy(destFile,partFile)
		partFile.Close()
		os.Remove(partFileName)
		bar.Add(1)
	}
	return nil
}